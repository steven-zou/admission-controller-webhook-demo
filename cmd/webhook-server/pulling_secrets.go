// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email string `json:"email"`
	Auth string `json:"auth"`
}

type dockerAuths struct {
	Auths map[string]*dockerAuth `json:"auths"`
}

func makeAuth(username, password string)[]byte{
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s",username, password)))
	dauth := &dockerAuths{
		Auths: map[string]*dockerAuth {
			"https://demo.goharbor.io/v2/":{
				Username: username,
				Password: password,
				Email: fmt.Sprintf("%s@goharbor.io", username),
				Auth: auth,
			},
		},
	}

	dt, _ := json.Marshal(dauth)
	return dt
}

func getClientSet() (*kubernetes.Clientset, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func makeSecret(namespace string, user string, pass string) error {
	clientset, err := getClientSet()
	if err != nil {
		return err
	}

	_, err =clientset.CoreV1().Secrets(namespace).Get(formatName(user), metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		authData := makeAuth(user, pass)
		log.Printf("auth data=%s", string(authData))

		// create new
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name: formatName(user),
				Labels: map[string]string{
					"owner": "tars",
				},
			},
			Type: corev1.SecretTypeDockerConfigJson,
			Data:map[string][]byte{
				".dockerconfigjson": authData,
			},
		}

		createdSec, err := clientset.CoreV1().Secrets(namespace).Create(secret)
		if err != nil {
			return err
		}

		log.Printf("Secret %s:%s@%s created", createdSec.Namespace, createdSec.Name, createdSec.Type)
	}

	// do nothing
	return nil
}

