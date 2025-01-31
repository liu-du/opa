// Copyright 2020 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package report provides functions to report OPA's version information to an external service and process the response.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/open-policy-agent/opa/keys"
	"github.com/open-policy-agent/opa/logging"

	"os"
	"time"

	"github.com/open-policy-agent/opa/plugins/rest"
	"github.com/open-policy-agent/opa/util"
	"github.com/open-policy-agent/opa/version"
)

// ExternalServiceURL is the base HTTP URL for a telemetry service.
// If not otherwise specified it will use the hard coded default.
//
// Override at build time via:
//
//    -ldflags "-X github.com/open-policy-agent/opa/internal/report.ExternalServiceURL=<url>"
//
// This will be overridden if the OPA_TELEMETRY_SERVICE_URL environment variable
// is provided.
var ExternalServiceURL = "https://telemetry.openpolicyagent.org"

// Reporter reports the version of the running OPA instance to an external service
type Reporter struct {
	body   map[string]string
	client rest.Client
}

// DataResponse represents the data returned by the external service
type DataResponse struct {
	Latest ReleaseDetails `json:"latest,omitempty"`
}

// ReleaseDetails holds information about the latest OPA release
type ReleaseDetails struct {
	Download      string `json:"download,omitempty"`       // link to download the OPA release
	ReleaseNotes  string `json:"release_notes,omitempty"`  // link to the OPA release notes
	LatestRelease string `json:"latest_release,omitempty"` // latest OPA released version
	OPAUpToDate   bool   `json:"opa_up_to_date,omitempty"` // is running OPA version greater than or equal to the latest released
}

// Options supplies parameters to the reporter.
type Options struct {
	Logger logging.Logger
}

// New returns an instance of the Reporter
func New(id string, opts Options) (*Reporter, error) {
	r := Reporter{}
	r.body = map[string]string{
		"id":      id,
		"version": version.Version,
	}

	url := os.Getenv("OPA_TELEMETRY_SERVICE_URL")
	if url == "" {
		url = ExternalServiceURL
	}

	restConfig := []byte(fmt.Sprintf(`{
		"url": %q,
	}`, url))

	client, err := rest.New(restConfig, map[string]*keys.Config{}, rest.Logger(opts.Logger))
	if err != nil {
		return nil, err
	}
	r.client = client

	return &r, nil
}

// SendReport sends the version report to the external service
func (r *Reporter) SendReport(ctx context.Context) (*DataResponse, error) {
	rCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := r.client.WithJSON(r.body).Do(rCtx, "POST", "/v1/version")
	if err != nil {
		return nil, err
	}

	defer util.Close(resp)

	switch resp.StatusCode {
	case http.StatusOK:
		if resp.Body != nil {
			var result DataResponse
			err := json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				return nil, err
			}
			return &result, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("server replied with HTTP %v", resp.StatusCode)
	}
}

// IsSet returns true if dr is populated.
func (dr *DataResponse) IsSet() bool {
	return dr != nil && dr.Latest.LatestRelease != "" && dr.Latest.Download != "" && dr.Latest.ReleaseNotes != ""
}

// Slice returns the dr as a slice of key-value string pairs. If dr is nil, this function returns an empty slice.
func (dr *DataResponse) Slice() [][2]string {

	if !dr.IsSet() {
		return nil
	}

	return [][2]string{
		{"Latest Upstream Version", strings.TrimPrefix(dr.Latest.LatestRelease, "v")},
		{"Download", dr.Latest.Download},
		{"Release Notes", dr.Latest.ReleaseNotes},
	}
}

// Pretty returns OPA release information in a human-readable format.
func (dr *DataResponse) Pretty() string {
	if !dr.IsSet() {
		return ""
	}

	pairs := dr.Slice()
	lines := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		lines = append(lines, fmt.Sprintf("%v: %v", pair[0], pair[1]))
	}

	return strings.Join(lines, "\n")
}
