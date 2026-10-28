/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/wutz/fileserver/api/v1alpha1"
	"github.com/wutz/fileserver/internal/config"
)

const (
	specTimeout  = 20 * time.Second
	specInterval = 200 * time.Millisecond
	testNS       = "forge-demo"
	// fileServerURL is the address of the default file server in testNS, given
	// testConfig (the ingress domain example.com on port 7443).
	fileServerURL = "https://files-" + testNS + ".example.com:7443"
)

// getObject returns a getter for the object, for use with Eventually.
func getObject(obj client.Object, name string) func() error {
	return func() error { return k8sClient.Get(ctx, client.ObjectKey{Namespace: testNS, Name: name}, obj) }
}

func createVolume(name string, ignored bool) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
	}
	if ignored {
		pvc.Labels = map[string]string{v1.LabelIgnore: "true"}
	}
	Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
}

var _ = Describe("FileServer", Ordered, func() {
	const name = v1.DefaultName
	deployment := "forgeai-files-" + name
	ingressName := deployment

	BeforeAll(func() {
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNS},
		}))).To(Succeed())
		createVolume("keep", false)
	})

	It("runs the file server while active and stops it when idle", func() {
		fs := &v1.FileServer{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS, Annotations: map[string]string{
				v1.AnnotationLastActive: time.Now().UTC().Format(time.RFC3339),
			}},
			Spec: v1.FileServerSpec{IdleTimeout: &metav1.Duration{Duration: 3 * time.Second}},
		}
		Expect(k8sClient.Create(ctx, fs)).To(Succeed())

		dep := &appsv1.Deployment{}
		Eventually(func() int32 {
			if getObject(dep, deployment)() != nil {
				return -1
			}
			return *dep.Spec.Replicas
		}, specTimeout, specInterval).Should(Equal(int32(1)))
		Expect(dep.Spec.Template.Annotations[annotationServedVolumes]).To(Equal("keep"))

		// The status is patched after the workload, so poll for the address.
		Eventually(func() string {
			_ = getObject(fs, name)()
			return fs.Status.URL
		}, specTimeout, specInterval).Should(Equal(fileServerURL))
		ing := &networkingv1.Ingress{}
		Expect(getObject(ing, ingressName)()).To(Succeed())
		Expect(ing.Spec.Rules[0].Host).To(Equal("files-" + testNS + ".example.com"))
		Expect(getObject(&corev1.Secret{}, v1.SecretName)()).To(Succeed())

		// No heartbeat: scaled to zero after the idle timeout.
		Eventually(func() int32 {
			if getObject(dep, deployment)() != nil {
				return -1
			}
			return *dep.Spec.Replicas
		}, specTimeout, specInterval).Should(Equal(int32(0)))
		Eventually(func() v1.FileServerPhase {
			_ = getObject(fs, name)()
			return fs.Status.Phase
		}, specTimeout, specInterval).Should(Equal(v1.FileServerIdle))

		// A heartbeat wakes it up again.
		Eventually(func() error {
			if err := getObject(fs, name)(); err != nil {
				return err
			}
			fs.Annotations[v1.AnnotationLastActive] = time.Now().UTC().Format(time.RFC3339)
			return k8sClient.Update(ctx, fs)
		}, specTimeout, specInterval).Should(Succeed())
		Eventually(func() int32 {
			if getObject(dep, deployment)() != nil {
				return -1
			}
			return *dep.Spec.Replicas
		}, specTimeout, specInterval).Should(Equal(int32(1)))
	})

	It("reports Failed without an ingress domain and publishes again once it is set", func() {
		fs := &v1.FileServer{}
		key := client.ObjectKey{Namespace: configNamespace, Name: config.DefaultConfigMapName}
		setConfig := func(data string) {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())
			cm.Data[config.ConfigKey] = data
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		}
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key, cm)).To(Succeed())
		original := cm.Data[config.ConfigKey]
		DeferCleanup(setConfig, original)

		setConfig(strings.Replace(original, "domain: example.com", `domain: ""`, 1))
		Eventually(func() v1.FileServerPhase {
			_ = getObject(fs, name)()
			return fs.Status.Phase
		}, specTimeout, specInterval).Should(Equal(v1.FileServerFailed))
		Expect(fs.Status.URL).To(BeEmpty())
		Expect(fs.Status.Conditions).To(ContainElement(HaveField("Message", ContainSubstring("ingress.domain"))))

		// The Ingress from the earlier spec is removed, and no reconcile applies it
		// again while the domain is unset. The Failed phase is patched after the
		// applies of its reconcile, so a stale apply cannot follow it.
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: ingressName, Namespace: testNS},
		}))).To(Succeed())
		Consistently(func() bool {
			return apierrors.IsNotFound(getObject(&networkingv1.Ingress{}, ingressName)())
		}, 2*time.Second, specInterval).Should(BeTrue())

		setConfig(original)
		Eventually(func() string {
			_ = getObject(fs, name)()
			return fs.Status.URL
		}, specTimeout, specInterval).Should(Equal(fileServerURL))
		Expect(fs.Status.Phase).NotTo(Equal(v1.FileServerFailed))
		Eventually(getObject(&networkingv1.Ingress{}, ingressName), specTimeout, specInterval).Should(Succeed())
	})

	It("stops serving a volume once it is ignored", func() {
		createVolume("shared", false)
		dep := &appsv1.Deployment{}
		Eventually(func() string {
			if getObject(dep, deployment)() != nil {
				return ""
			}
			return dep.Spec.Template.Annotations[annotationServedVolumes]
		}, specTimeout, specInterval).Should(Equal("keep,shared"))

		// A running file server pod mounts the volume.
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: deployment + "-0", Namespace: testNS, Labels: map[string]string{
				labelComponent: componentFileServer, labelInstance: deployment,
			}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: fileServerContainer, Image: "dufs"}},
				Volumes: []corev1.Volume{{Name: "v-shared", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "shared"},
				}}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.Phase = corev1.PodRunning
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		pvc := &corev1.PersistentVolumeClaim{}
		Expect(getObject(pvc, "shared")()).To(Succeed())
		if pvc.Labels == nil {
			pvc.Labels = map[string]string{}
		}
		pvc.Labels[v1.LabelIgnore] = "true"
		Expect(k8sClient.Update(ctx, pvc)).To(Succeed())
		Eventually(func() string {
			if getObject(dep, deployment)() != nil {
				return ""
			}
			return dep.Spec.Template.Annotations[annotationServedVolumes]
		}, specTimeout, specInterval).Should(Equal("keep"))
		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
	})
})
