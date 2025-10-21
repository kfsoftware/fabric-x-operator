/*
Copyright 2025.

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

package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// GenerateCoreYAML generates the core.yaml content from the typed configuration
func (e *Endorser) GenerateCoreYAML() (string, error) {
	coreConfig := e.Spec.Core

	// Build the core configuration map
	config := make(map[string]interface{})

	// Logging configuration
	if coreConfig.Logging != nil {
		config["logging"] = map[string]interface{}{
			"spec":   coreConfig.Logging.Spec,
			"format": coreConfig.Logging.Format,
		}
	}

	// FSC configuration
	fscConfig := map[string]interface{}{
		"id": coreConfig.FSC.ID,
		"identity": map[string]interface{}{
			"cert": map[string]interface{}{
				"file": "/var/hyperledger/fabric/msp/signcerts/cert.pem",
			},
			"key": map[string]interface{}{
				"file": "/var/hyperledger/fabric/msp/keystore/key.pem",
			},
		},
		"p2p": map[string]interface{}{
			"listenAddress": coreConfig.FSC.P2P.ListenAddress,
		},
	}

	// Add P2P type if specified
	if coreConfig.FSC.P2P.Type != "" {
		fscConfig["p2p"].(map[string]interface{})["type"] = coreConfig.FSC.P2P.Type
	}

	// Add P2P options if specified
	if coreConfig.FSC.P2P.Opts != nil && coreConfig.FSC.P2P.Opts.Routing != nil {
		opts := make(map[string]interface{})
		routing := make(map[string]interface{})

		// Embed inline routing configuration if specified
		if coreConfig.FSC.P2P.Opts.Routing.Inline != nil && len(coreConfig.FSC.P2P.Opts.Routing.Inline.Raw) > 0 {
			var inlineConfig map[string]interface{}
			if err := yaml.Unmarshal(coreConfig.FSC.P2P.Opts.Routing.Inline.Raw, &inlineConfig); err == nil {
				routing = inlineConfig
			}
		} else if coreConfig.FSC.P2P.Opts.Routing.Path != "" {
			routing["path"] = coreConfig.FSC.P2P.Opts.Routing.Path
		}

		opts["routing"] = routing
		fscConfig["p2p"].(map[string]interface{})["opts"] = opts
	}

	// Add persistences if specified
	if len(coreConfig.FSC.Persistences) > 0 {
		persistences := make(map[string]interface{})
		for name, pc := range coreConfig.FSC.Persistences {
			persistence := map[string]interface{}{
				"type": pc.Type,
			}
			if len(pc.Opts) > 0 {
				persistence["opts"] = pc.Opts
			}
			persistences[name] = persistence
		}
		fscConfig["persistences"] = persistences
	}

	// Add endpoint resolvers if specified
	if coreConfig.FSC.Endpoint != nil && len(coreConfig.FSC.Endpoint.Resolvers) > 0 {
		resolvers := make([]map[string]interface{}, 0, len(coreConfig.FSC.Endpoint.Resolvers))
		for _, resolver := range coreConfig.FSC.Endpoint.Resolvers {
			r := map[string]interface{}{
				"name": resolver.Name,
			}

			if resolver.Identity != nil {
				identity := make(map[string]interface{})
				if resolver.Identity.ID != "" {
					identity["id"] = resolver.Identity.ID
				}
				// Generate file path for identity certificate
				// If SecretRef is specified, use a standard mount path
				// If Path is specified, use it directly
				if resolver.Identity.SecretRef != nil {
					// Use standard path where secret will be mounted
					identity["path"] = fmt.Sprintf("/var/hyperledger/fabric/resolvers/%s/cert.pem", resolver.Name)
				} else if resolver.Identity.Path != "" {
					identity["path"] = resolver.Identity.Path
				}
				r["identity"] = identity
			}

			if len(resolver.Addresses) > 0 {
				r["addresses"] = resolver.Addresses
			}

			resolvers = append(resolvers, r)
		}
		fscConfig["endpoint"] = map[string]interface{}{
			"resolvers": resolvers,
		}
	}

	config["fsc"] = fscConfig

	// Fabric configuration
	if coreConfig.Fabric != nil && coreConfig.Fabric.Enabled {
		fabricConfig := map[string]interface{}{
			"enabled": true,
		}

		if coreConfig.Fabric.Default != nil {
			defaultConfig := make(map[string]interface{})

			if coreConfig.Fabric.Default.Driver != "" {
				defaultConfig["driver"] = coreConfig.Fabric.Default.Driver
			}

			if coreConfig.Fabric.Default.DefaultMSP != "" {
				defaultConfig["defaultMSP"] = coreConfig.Fabric.Default.DefaultMSP
			}

			if len(coreConfig.Fabric.Default.MSPs) > 0 {
				msps := make([]map[string]interface{}, 0, len(coreConfig.Fabric.Default.MSPs))
				for _, msp := range coreConfig.Fabric.Default.MSPs {
					m := map[string]interface{}{
						"id":    msp.ID,
						"mspID": msp.MSPID,
					}
					if msp.MSPType != "" {
						m["mspType"] = msp.MSPType
					}
					if msp.Path != "" {
						m["path"] = msp.Path
					}
					msps = append(msps, m)
				}
				defaultConfig["msps"] = msps
			}

			if coreConfig.Fabric.Default.TLS != nil {
				defaultConfig["tls"] = map[string]interface{}{
					"enabled": coreConfig.Fabric.Default.TLS.Enabled,
				}
			}

			if len(coreConfig.Fabric.Default.Peers) > 0 {
				peers := make([]map[string]interface{}, 0, len(coreConfig.Fabric.Default.Peers))
				for _, peer := range coreConfig.Fabric.Default.Peers {
					p := map[string]interface{}{
						"address": peer.Address,
					}
					if peer.Usage != "" {
						p["usage"] = peer.Usage
					}
					peers = append(peers, p)
				}
				defaultConfig["peers"] = peers
			}

			if len(coreConfig.Fabric.Default.QueryService) > 0 {
				queryServices := make([]map[string]interface{}, 0, len(coreConfig.Fabric.Default.QueryService))
				for _, qs := range coreConfig.Fabric.Default.QueryService {
					queryServices = append(queryServices, map[string]interface{}{
						"address": qs.Address,
					})
				}
				defaultConfig["queryService"] = queryServices
			}

			if len(coreConfig.Fabric.Default.Channels) > 0 {
				channels := make([]map[string]interface{}, 0, len(coreConfig.Fabric.Default.Channels))
				for _, ch := range coreConfig.Fabric.Default.Channels {
					c := map[string]interface{}{
						"name": ch.Name,
					}
					if ch.Default {
						c["default"] = true
					}
					channels = append(channels, c)
				}
				defaultConfig["channels"] = channels
			}

			fabricConfig["default"] = defaultConfig
		}

		config["fabric"] = fabricConfig
	}

	// Token configuration
	if coreConfig.Token != nil && coreConfig.Token.Enabled {
		tokenConfig := map[string]interface{}{
			"enabled": true,
		}

		if len(coreConfig.Token.TMS) > 0 {
			tms := make(map[string]interface{})
			for name, tmsConfig := range coreConfig.Token.TMS {
				tc := make(map[string]interface{})

				if tmsConfig.Network != "" {
					tc["network"] = tmsConfig.Network
				}
				if tmsConfig.Channel != "" {
					tc["channel"] = tmsConfig.Channel
				}
				if tmsConfig.Namespace != "" {
					tc["namespace"] = tmsConfig.Namespace
				}
				if tmsConfig.Driver != "" {
					tc["driver"] = tmsConfig.Driver
				}

				if tmsConfig.Services != nil && tmsConfig.Services.Network != nil && tmsConfig.Services.Network.Fabric != nil {
					services := map[string]interface{}{
						"network": map[string]interface{}{
							"fabric": map[string]interface{}{},
						},
					}

					if tmsConfig.Services.Network.Fabric.FSCEndorsement != nil {
						fscEndorsement := make(map[string]interface{})
						fscEnd := tmsConfig.Services.Network.Fabric.FSCEndorsement

						if fscEnd.Endorser {
							fscEndorsement["endorser"] = true
						}
						if fscEnd.ID != "" {
							fscEndorsement["id"] = fscEnd.ID
						}
						if fscEnd.Policy != nil {
							policy := map[string]interface{}{
								"type": fscEnd.Policy.Type,
							}
							if fscEnd.Policy.Threshold > 0 {
								policy["threshold"] = fscEnd.Policy.Threshold
							}
							fscEndorsement["policy"] = policy
						}
						if len(fscEnd.Endorsers) > 0 {
							fscEndorsement["endorsers"] = fscEnd.Endorsers
						}

						services["network"].(map[string]interface{})["fabric"].(map[string]interface{})["fsc_endorsement"] = fscEndorsement
					}

					tc["services"] = services
				}

				// Add public parameters if specified
				if tmsConfig.PublicParameters != nil && tmsConfig.PublicParameters.Path != "" {
					tc["publicParameters"] = map[string]interface{}{
						"path": tmsConfig.PublicParameters.Path,
					}
				}

				tms[name] = tc
			}
			tokenConfig["tms"] = tms
		}

		config["token"] = tokenConfig
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal core config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// GetCoreConfigSecretName returns the name of the secret that will contain the core.yaml
func (e *Endorser) GetCoreConfigSecretName() string {
	return fmt.Sprintf("%s-core-config", e.Name)
}

// GetCertSecretName returns the name of the secret that will contain the certificates
func (e *Endorser) GetCertSecretName() string {
	return fmt.Sprintf("%s-certs", e.Name)
}

// GetServiceName returns the name of the Kubernetes service for this endorser
func (e *Endorser) GetServiceName() string {
	return fmt.Sprintf("%s-service", e.Name)
}

// GetDeploymentName returns the name of the Kubernetes deployment for this endorser
func (e *Endorser) GetDeploymentName() string {
	return e.Name
}

// GetDefaultImage returns the default image for the endorser if not specified
func (e *Endorser) GetDefaultImage() string {
	if e.Spec.Image != "" {
		return e.Spec.Image
	}
	return "hyperledger/fabric-smart-client"
}

// GetDefaultVersion returns the default version for the endorser if not specified
func (e *Endorser) GetDefaultVersion() string {
	if e.Spec.Version != "" {
		return e.Spec.Version
	}
	return "latest"
}

// GetFullImage returns the full image name with tag
func (e *Endorser) GetFullImage() string {
	return fmt.Sprintf("%s:%s", e.GetDefaultImage(), e.GetDefaultVersion())
}
