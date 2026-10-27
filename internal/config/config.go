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

// Package config is the operator's own configuration, read from a ConfigMap.
// The Helm chart renders it from the chart values; see charts/fileserver-operator.
package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// ConfigKey is the key of the ConfigMap that holds the YAML document.
	ConfigKey = "config.yaml"
	// DefaultConfigMapName is the ConfigMap read when none is given.
	DefaultConfigMapName = "fileserver-operator"
	// DefaultImage is the dufs image served when the config does not name one.
	DefaultImage = "docker.io/sigoden/dufs:v0.46.0"
	// reloadAfter bounds how stale the cached config can be. Changes of the
	// ConfigMap invalidate the cache at once, so this is only a backstop.
	reloadAfter = 15 * time.Second
)

// Config is the operator configuration.
type Config struct {
	Ingress    Ingress    `json:"ingress"`
	FileServer FileServer `json:"fileServer"`
}

// Ingress is how the file servers are exposed.
type Ingress struct {
	ClassName string `json:"className,omitempty"`
	// Domain is the wildcard DNS suffix, e.g. dev1.example.com. Empty means
	// the file servers cannot be published.
	Domain string `json:"domain,omitempty"`
	// Port is the external HTTPS port (443 is omitted from URLs).
	Port int `json:"port,omitempty"`
	// TLSSecret is an optional certificate Secret in each file server's
	// namespace; empty uses the ingress controller default certificate.
	TLSSecret string `json:"tlsSecret,omitempty"`
}

// URL returns the external URL of host <sub>.<domain>.
func (i Ingress) URL(sub string) string {
	u := "https://" + i.Host(sub)
	if i.Port != 0 && i.Port != 443 {
		u += fmt.Sprintf(":%d", i.Port)
	}
	return u
}

// Host returns <sub>.<domain>.
func (i Ingress) Host(sub string) string { return sub + "." + i.Domain }

// FileServer configures the on-demand file transfer service.
type FileServer struct {
	// Image is the dufs image.
	Image string `json:"image,omitempty"`
	// IdleTimeout stops a file server after no activity for this long.
	IdleTimeout metav1.Duration `json:"idleTimeout,omitzero"`
	// HostPrefix builds the host <prefix><namespace>.<ingress domain>.
	HostPrefix string `json:"hostPrefix,omitempty"`
}

// Parse decodes the YAML document and fills in the defaults.
func Parse(data []byte) (*Config, error) {
	c := &Config{}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Ingress.Port == 0 {
		c.Ingress.Port = 443
	}
	if c.FileServer.Image == "" {
		c.FileServer.Image = DefaultImage
	}
	if c.FileServer.HostPrefix == "" {
		c.FileServer.HostPrefix = "files-"
	}
	if c.FileServer.IdleTimeout.Duration == 0 {
		c.FileServer.IdleTimeout.Duration = 30 * time.Minute
	}
	return c, nil
}

// Loader reads the configuration ConfigMap and caches it for a short time.
type Loader struct {
	Reader    client.Reader
	Namespace string
	// Name is the ConfigMap; empty means DefaultConfigMapName.
	Name string

	mu   sync.Mutex
	cfg  *Config
	time time.Time
}

// ConfigMapName returns the ConfigMap the loader reads.
func (l *Loader) ConfigMapName() string {
	if l.Name == "" {
		return DefaultConfigMapName
	}
	return l.Name
}

// Get returns the cached config, reloading it after reloadAfter.
func (l *Loader) Get(ctx context.Context) (*Config, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cfg != nil && time.Since(l.time) < reloadAfter {
		return l.cfg, nil
	}
	cm := &corev1.ConfigMap{}
	if err := l.Reader.Get(ctx, types.NamespacedName{Namespace: l.Namespace, Name: l.ConfigMapName()}, cm); err != nil {
		return nil, fmt.Errorf("get config %s/%s: %w", l.Namespace, l.ConfigMapName(), err)
	}
	cfg, err := Parse([]byte(cm.Data[ConfigKey]))
	if err != nil {
		return nil, err
	}
	l.cfg, l.time = cfg, time.Now()
	return cfg, nil
}

// Invalidate drops the cached config, so that the next Get reads the
// ConfigMap again. Controllers call it when the ConfigMap changes, so that a
// change takes effect at once instead of after the cache expires.
func (l *Loader) Invalidate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg = nil
}
