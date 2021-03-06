/*
Copyright (c) 2019 StackRox Inc.

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

package main

import (
	"fmt"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	tlsDir      = `/run/secrets/tls`
	tlsCertFile = `tls.crt`
	tlsKeyFile  = `tls.key`
)

var (
	podResource = metav1.GroupVersionResource{Version: "v1", Resource: "pods"}
	registry = "demo.goharbor.io/tars"
)

const (
	pullUser = "robot$foradmin"
	pullSecret = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1ODY0MDMzOTgsImlhdCI6MTU4MzgxMTM5OCwiaXNzIjoiaGFyYm9yLXRva2VuLWRlZmF1bHRJc3N1ZXIiLCJpZCI6MzIsInBpZCI6NSwiYWNjZXNzIjpbeyJSZXNvdXJjZSI6Ii9wcm9qZWN0LzUvcmVwb3NpdG9yeSIsIkFjdGlvbiI6InB1c2giLCJFZmZlY3QiOiIifSx7IlJlc291cmNlIjoiL3Byb2plY3QvNS9oZWxtLWNoYXJ0IiwiQWN0aW9uIjoicmVhZCIsIkVmZmVjdCI6IiJ9LHsiUmVzb3VyY2UiOiIvcHJvamVjdC81L2hlbG0tY2hhcnQtdmVyc2lvbiIsIkFjdGlvbiI6ImNyZWF0ZSIsIkVmZmVjdCI6IiJ9XX0.iFK7pC4OhrQoxGJ2U49IPxDPlgRXOVSsX8QEtM4bfvU9ntmuA2lTiPJ9bWWLSGKyaVUY3xPOeyTukXHE9-vmjLZ0FMJIxdo1RTtqVDxtQT0rOrw3R5qlCnsQ5PrqLoTMOawVy7QfGYF52Xcvi44TQGgwn2ZBv8Jn4QhE_o0g7OfSx0FGAJEvYcTi9_MMuIMLrGCtzh-5QlB55MUc6GfGAE5n8T0K-4-s75yi1ada6RiXqRd9WHzPWlWPc9PhW0HdeIYpH1yXQ7W086BHB8-OcC8yRUaH349G-ReRzVSVhvCXoWZXEjPRCpPzr07Yene-EnpQJoC9kGLC6Iya15bmQQmjjwqWEN5gLQaz_bNnJmIlTBw_O6MbidkC1nVCLnikwdYb6CjS48F7sDsznG7o3koJl9MnheLy3GHHPrdt-AxqA07J8CMWuv6FmtgoXV2DB74aq5LcxCWsiNTV0IccSLYl-jve_ssYiaCwkVHEw2FqX2am7VuwRIK6NNAeMbsw3QzXv9QwbYGiqAcpD7ZIYrirVSXjfy7U4JvpEd_rYw7i5LuhHy2zZbCQ6n_jId6yl3KFK7Zzj2Zt9av6XU0_zpU28dToGZFFi5ytRx4tMQNE5ZHcFPjC0fFrsrfVy8nwmeN8rMU_V82h4ZhV4oWfIbVWHcnnKKb3sYUCGmoS3p4"
)

func containDomain(domain string) bool {
	RegExp := regexp.MustCompile(`^(([a-zA-Z]{1})|([a-zA-Z]{1}[a-zA-Z]{1})|([a-zA-Z]{1}[0-9]{1})|([0-9]{1}[a-zA-Z]{1})|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z
 ]{2,3})\/`)

	return RegExp.MatchString(domain)
}

func setImage(image string) string {
	// image FQDN:
	// registry/namespace/repository:tag
	// e.g: docker.io/library/busybox:latest

	// Image: busybox:latest
	// stevenzou/busybox:latest
	// busybox
	// stevenzou/busybox

	img := image
	if !containDomain(image){
		img = fmt.Sprintf("%s/%s", registry, img)
	}

	if strings.LastIndex(img,":") == -1 {
		img = fmt.Sprintf("%s:%s", img, "latest")
	}

	return img
}

// applySecurityDefaults implements the logic of our example admission controller webhook. For every pod that is created
// (outside of Kubernetes namespaces), it first checks if `runAsNonRoot` is set. If it is not, it is set to a default
// value of `false`. Furthermore, if `runAsUser` is not set (and `runAsNonRoot` was not initially set), it defaults
// `runAsUser` to a value of 1234.
//
// To demonstrate how requests can be rejected, this webhook further validates that the `runAsNonRoot` setting does
// not conflict with the `runAsUser` setting - i.e., if the former is set to `true`, the latter must not be `0`.
// Note that we combine both the setting of defaults and the check for potential conflicts in one webhook; ideally,
// the latter would be performed in a validating webhook admission controller.
func applySecurityDefaults(req *v1beta1.AdmissionRequest) ([]patchOperation, error) {
	// This handler should only get called on Pod objects as per the MutatingWebhookConfiguration in the YAML file.
	// However, if (for whatever reason) this gets invoked on an object of a different kind, issue a log message but
	// let the object request pass through otherwise.
	if req.Resource != podResource {
		log.Printf("expect resource to be %s", podResource)
		return nil, nil
	}

	// Parse the Pod object.
	raw := req.Object.Raw

	log.Printf("Pod coming: %s", string(raw))

	pod := corev1.Pod{}

	if _, _, err := universalDeserializer.Decode(raw, nil, &pod); err != nil {
		return nil, fmt.Errorf("could not deserialize pod object: %v", err)
	}

	// Create patch operations to apply sensible defaults, if those options are not set explicitly.
	var patches []patchOperation

	// Check the images
	for i, c := range pod.Spec.Containers{
		if len(c.Image) > 0 {
			img := setImage(c.Image)

			log.Printf("Mutate image of main containers[%d]: %s\n", i, img)

			patches = append(patches, patchOperation{
				Op:    "replace",
				Path:  fmt.Sprintf("/spec/containers/%d/image",i),
				// The value must not be true if runAsUser is set to 0, as otherwise we would create a conflicting
				// configuration ourselves.
				Value: img,
			})
		}
	}

	for i, c := range pod.Spec.InitContainers {
		if len(c.Image) > 0 {
			img := setImage(c.Image)
			log.Printf("Mutate image of init containers[%d]: %s\n", i, img)
			patches = append(patches, patchOperation{
				Op:    "replace",
				Path:  fmt.Sprintf("/spec/initContainers/%d/image",i),
				// The value must not be true if runAsUser is set to 0, as otherwise we would create a conflicting
				// configuration ourselves.
				Value: img,
			})
		}
	}

	// inject image pulling secret
	// imagePullSecrets
	if err := makeSecret(req.Namespace, pullUser, pullSecret); err!=nil {
		log.Printf("Making secret error: %s", err)
	}else{
		log.Print("Append image pulling secret...")

		if pod.Spec.ImagePullSecrets == nil {
			log.Print("Create imagePullSecrets array...")

			patches = append(patches, patchOperation{
				Op:    "add",
				Path:  "/spec/imagePullSecrets",
				// The value must not be true if runAsUser is set to 0, as otherwise we would create a conflicting
				// configuration ourselves.
				Value: []corev1.LocalObjectReference{
					{
						Name: formatName(pullUser),
					},
				},
			})
		}else{
			patches = append(patches, patchOperation{
				Op:    "add",
				Path:  "/spec/imagePullSecrets/-",
				// The value must not be true if runAsUser is set to 0, as otherwise we would create a conflicting
				// configuration ourselves.
				Value: corev1.LocalObjectReference{
					Name: formatName(pullUser),
				},
			})
		}
	}

	return patches, nil
}

func formatName(name string) string {
	return fmt.Sprintf("image.pulling.secret.%s", strings.Replace(name,"$", ".", 1))
}

func main() {
	certPath := filepath.Join(tlsDir, tlsCertFile)
	keyPath := filepath.Join(tlsDir, tlsKeyFile)

	mux := http.NewServeMux()
	mux.Handle("/mutate", admitFuncHandler(applySecurityDefaults))
	server := &http.Server{
		// We listen on port 8443 such that we do not need root privileges or extra capabilities for this server.
		// The Service object will take care of mapping this port to the HTTPS port 443.
		Addr:    ":8443",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServeTLS(certPath, keyPath))
}
