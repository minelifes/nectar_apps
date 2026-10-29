package kube

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// K3sKubeconfig is where k3s writes the admin kubeconfig.
const K3sKubeconfig = "/etc/rancher/k3s/k3s.yaml"

type kubeconfig struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			CertificateAuthority     string `yaml:"certificate-authority"`
			InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
			ClientCertificate     string `yaml:"client-certificate"`
			ClientKey             string `yaml:"client-key"`
			Token                 string `yaml:"token"`
		} `yaml:"user"`
	} `yaml:"users"`
}

// FromKubeconfig returns a client for the current context of a kubeconfig file (client certificate or
// static token auth, as written by k3s). Used by the host-side `nectar-apps adopt` commands.
func FromKubeconfig(path string) (*Client, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	return parseKubeconfig(raw)
}

func parseKubeconfig(raw []byte) (*Client, error) {
	var kc kubeconfig
	if err := yaml.Unmarshal(raw, &kc); err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	if len(kc.Contexts) == 0 || len(kc.Clusters) == 0 {
		return nil, errors.New("kubeconfig: no context or cluster")
	}
	ctxName := kc.CurrentContext
	if ctxName == "" {
		ctxName = kc.Contexts[0].Name
	}
	var clusterName, userName string
	for _, c := range kc.Contexts {
		if c.Name == ctxName {
			clusterName, userName = c.Context.Cluster, c.Context.User
		}
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	server := ""
	for _, c := range kc.Clusters {
		if c.Name != clusterName {
			continue
		}
		server = c.Cluster.Server
		ca, err := dataOrFile(c.Cluster.CertificateAuthorityData, c.Cluster.CertificateAuthority)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig CA: %w", err)
		}
		if len(ca) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(ca) {
				return nil, errors.New("kubeconfig: invalid CA certificate")
			}
			tlsCfg.RootCAs = pool
		}
		tlsCfg.InsecureSkipVerify = c.Cluster.InsecureSkipTLSVerify
	}
	if server == "" {
		return nil, fmt.Errorf("kubeconfig: cluster %q of context %q not found", clusterName, ctxName)
	}
	cl := &Client{base: strings.TrimRight(server, "/")}
	for _, u := range kc.Users {
		if u.Name != userName {
			continue
		}
		crt, err := dataOrFile(u.User.ClientCertificateData, u.User.ClientCertificate)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig client cert: %w", err)
		}
		key, err := dataOrFile(u.User.ClientKeyData, u.User.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig client key: %w", err)
		}
		if len(crt) > 0 {
			pair, err := tls.X509KeyPair(crt, key)
			if err != nil {
				return nil, fmt.Errorf("kubeconfig client certificate: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{pair}
		}
		cl.token = u.User.Token
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	cl.http = &http.Client{Transport: tr, Timeout: 30 * time.Second}
	return cl, nil
}

func dataOrFile(data, file string) ([]byte, error) {
	if data != "" {
		return base64.StdEncoding.DecodeString(strings.TrimSpace(data))
	}
	if file != "" {
		return os.ReadFile(file)
	}
	return nil, nil
}
