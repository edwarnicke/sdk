// Copyright (c) 2020 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package interpose provides a NetworkServiceServer chain element that tracks local Cross connect Endpoints and call them first
// their unix file socket as the clienturl.ClientURL(ctx) used to connect to them.
package interpose

import (
	"context"
	"net/url"
	"sync"

	"github.com/networkservicemesh/sdk/pkg/registry/common/interpose"
	interpose_tools "github.com/networkservicemesh/sdk/pkg/tools/interpose"

	"github.com/pkg/errors"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/networkservicemesh/sdk/pkg/networkservice/core/trace"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/registry"

	"github.com/networkservicemesh/sdk/pkg/networkservice/common/clienturl"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
)

type interposeServer struct {
	// Map of names -> *registry.NetworkServiceEndpoint for local bypass to file endpoints
	endpoints interpose_tools.Map

	activeConnection sync.Map

	name string
}

type connectionInfo struct {
	endpointURL     *url.URL
	interposeNSEURL *url.URL
	closingNSE      bool
}

// NewServer - creates a NetworkServiceServer that tracks locally registered CrossConnect Endpoints and on first Request forward to cross conenct nse
//				one by one and if request came back from cross nse, it will connect to a proper next client endpoint.
//             - server - *registry.NetworkServiceRegistryServer.  Since registry.NetworkServiceRegistryServer is an interface
//                        (and thus a pointer) *registry.NetworkServiceRegistryServer is a double pointer.  Meaning it
//                        points to a place that points to a place that implements registry.NetworkServiceRegistryServer
//                        This is done so that we can return a registry.NetworkServiceRegistryServer chain element
//                        while maintaining the NewServer pattern for use like anything else in a chain.
//                        The value in *server must be included in the registry.NetworkServiceRegistryServer listening
//                        so it can capture the registrations.
func NewServer(name string, registryServer *registry.NetworkServiceEndpointRegistryServer) networkservice.NetworkServiceServer {
	rv := &interposeServer{
		name: name,
	}
	*registryServer = interpose.NewNetworkServiceRegistryServer(&rv.endpoints)
	return rv
}

func (l *interposeServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (result *networkservice.Connection, err error) {
	// Check if there is no active connection, we need to replace endpoint url with forwarder url
	conn := request.GetConnection()
	ind := conn.GetPath().GetIndex() // It is designed to be used inside Endpoint, so current index is Endpoint already
	connID := conn.GetId()

	if len(conn.GetPath().GetPathSegments()) == 0 || ind <= 0 {
		return nil, errors.Errorf("path segment doesn't have a client or cross connect nse identity")
	}
	// We need to find an Id from path to match active connection request.
	clientConnID := l.getConnectionID(conn)

	// We came from client, so select cross nse and go to it.
	clientURL := clienturl.ClientURL(ctx)

	connInfoRaw, ok := l.activeConnection.Load(clientConnID)
	if !ok {
		if connID != clientConnID {
			return nil, errors.Errorf("connection id should match current path segment id")
		}

		// Iterate over all cross connect NSEs to check one with passed state.

		l.endpoints.Range(func(key string, value *registry.NetworkServiceEndpoint) bool {
			crossNSEURL, _ := url.Parse(value.Url)
			crossCTX := clienturl.WithClientURL(ctx, crossNSEURL)

			// Store client connection and selected cross connection URL.
			_, _ = l.activeConnection.LoadOrStore(conn.Id, &connectionInfo{
				endpointURL:     clientURL,
				interposeNSEURL: crossNSEURL,
			})
			result, err = next.Server(crossCTX).Request(crossCTX, request)
			if err != nil {
				trace.Log(ctx).Errorf("failed to request cross NSE %v err: %v", crossNSEURL, err)
				return true
			}
			// If all is ok, stop iterating.
			return false
		})
		if result != nil {
			return result, nil
		}
		return nil, errors.Errorf("all cross NSE failed to connect to endpoint %v connection: %v", clientURL, conn)
	}

	// Go to endpoint URL if it matches one we had on previous step.
	connInfo := connInfoRaw.(*connectionInfo)
	if clientURL != connInfo.endpointURL {
		return nil, errors.Errorf("new selected endpoint URL %v doesn't match endpoint URL selected before interpose NSE %v", clientURL, connInfo.endpointURL)
	}
	crossCTX := clienturl.WithClientURL(ctx, connInfo.endpointURL)
	return next.Server(crossCTX).Request(crossCTX, request)
}

func (l *interposeServer) getConnectionID(conn *networkservice.Connection) string {
	id := ""
	for i := conn.GetPath().GetIndex(); i > 0; i-- {
		if conn.GetPath().GetPathSegments()[i].Name == l.name {
			id = conn.GetPath().GetPathSegments()[i].Id
		}
	}
	return id
}

func (l *interposeServer) Close(ctx context.Context, conn *networkservice.Connection) (*empty.Empty, error) {
	// We need to find an Id from path to match active connection request.
	id := l.getConnectionID(conn)

	// We came from cross nse, we need to go to proper endpoint
	connInfoRaw, ok := l.activeConnection.Load(id)
	if !ok {
		return nil, errors.Errorf("no active connection found but we called from cross NSE %v", conn)
	}
	connInfo := connInfoRaw.(*connectionInfo)
	if !connInfo.closingNSE {
		// If not closing NSE go to cross connect
		connInfo.closingNSE = true
		crossCTX := clienturl.WithClientURL(ctx, connInfo.interposeNSEURL)
		return next.Server(crossCTX).Close(crossCTX, conn)
	}
	// We are closing NSE, go to endpoint here.
	crossCTX := clienturl.WithClientURL(ctx, connInfo.endpointURL)
	return next.Server(crossCTX).Close(crossCTX, conn)
}

func (l *interposeServer) LoadOrStore(name string, endpoint *registry.NetworkServiceEndpoint) (*registry.NetworkServiceEndpoint, bool) {
	r, ok := l.endpoints.LoadOrStore(name, endpoint)
	if ok {
		return r, true
	}
	return nil, false
}

func (l *interposeServer) Delete(name string) {
	l.endpoints.Delete(name)
}
