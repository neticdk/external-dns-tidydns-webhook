/*
Copyright 2024 Netic A/S.

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
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type webhook interface {
	negociate(w http.ResponseWriter, req *http.Request)
	getRecords(w http.ResponseWriter, req *http.Request)
	adjustEndpoints(w http.ResponseWriter, req *http.Request)
	applyChanges(w http.ResponseWriter, req *http.Request)
}

type tidyWebhook struct {
	provider *tidyProvider
}

const (
	headerKey   = "Content-Type"
	headerValue = "application/external.dns.webhook+json;version=1"
)

func newWebhook(p *tidyProvider) webhook {
	return &tidyWebhook{p}
}

// Return list of domainfilters
func (wh *tidyWebhook) negociate(w http.ResponseWriter, req *http.Request) {
	w.Header().Set(headerKey, headerValue)

	// Encode response
	resp, err := wh.provider.GetDomainFilter().MarshalJSON()
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(resp)
}

// Return list of all records using the External-DNS Endpoint list format
func (wh *tidyWebhook) getRecords(w http.ResponseWriter, req *http.Request) {
	w.Header().Set(headerKey, headerValue)

	// Get all tidy endpoints
	endpoints, err := wh.provider.Records(context.Background())
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// encode response
	resp, err := json.Marshal(endpoints)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(resp)
}

// Recieve a list of proposed endpoints, including endpoints that will later be
// filtered out by the domainfilter, and modify them so they are consumable to
// TidyDNS before returning them. This is to inform External-DNS how the records
// will look when saved so they can be checked for correctness.
func (wh *tidyWebhook) adjustEndpoints(w http.ResponseWriter, req *http.Request) {
	w.Header().Set(headerKey, headerValue)

	// Read request
	msg, err := io.ReadAll(req.Body)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Map request body to endpoint list
	endpoints := []*endpoint.Endpoint{}
	if err = json.Unmarshal(msg, &endpoints); err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Process request
	adjustedEndpoints, err := wh.provider.AdjustEndpoints(endpoints)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// encode response
	resp, err := json.Marshal(adjustedEndpoints)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(resp)
}

// Consume a struct with 4 lists. Endpoints to create and delete, and a 2 lists
// representing changes to endpoints. The two changes lists are of equal length
// and represent the before and after spec of each endpoint to be changed.
func (wh *tidyWebhook) applyChanges(w http.ResponseWriter, req *http.Request) {
	w.Header().Set(headerKey, headerValue)

	// Read request
	msg, err := io.ReadAll(req.Body)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Map request body to endpoint list
	changes := &plan.Changes{}
	if err = json.Unmarshal(msg, changes); err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Process request
	err = wh.provider.ApplyChanges(context.Background(), changes)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// The expected return code and content is left undocumented by External-DNS
	// at this time but
	// https://github.com/kubernetes-sigs/external-dns/blob/9fb831e97f77b31789df8d837e93f36a6e785562/provider/webhook/webhook.go#L229
	// reveals that it excepts an empty return with code 204 (no content) when
	// calling POST /records
	w.WriteHeader(http.StatusNoContent)
}
