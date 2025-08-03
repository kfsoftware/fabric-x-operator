package ca

import (
	"bytes"
	"text/template"
)

// CAConfigTemplate is the Go template for the main CA configuration
const CAConfigTemplate = `#############################################################################
#   This is a configuration file for the fabric-ca-server command.
#############################################################################
version: "1.4.9"
# Server's listening port
port: 7054
# Enables debug logging
debug: {{.Debug}}
# Size limit of an acceptable CRL in bytes (default: 512000)
crlsizelimit: {{.CLRSizeLimit}}
#############################################################################
#  TLS section
#############################################################################
tls:
  # Enable TLS
  enabled: true
  # TLS for the server's listening port
  certfile: /var/hyperledger/tls/secret/tls.crt
  keyfile: /var/hyperledger/tls/secret/tls.key
  clientauth:
    # Supported types: NoClientCert, RequestClientCert, RequireAnyClientCert, VerifyClientCertIfGiven and RequireAndVerifyClientCert.
    type: noclientcert
    # List of root certificate authorities used when verifying client certificates
    certfiles:
#############################################################################
#  The CA section contains information related to the Certificate Authority
#  including the name of the CA, which should be unique for all members
#  of a blockchain network.  It also includes the key and certificate files
#  used when issuing enrollment certificates (ECerts) and transaction
#  certificates (TCerts).
#  The chainfile (if it exists) contains the certificate chain which
#  should be trusted for this CA, where the 1st in the chain is always the
#  root CA certificate.
#############################################################################
ca:
  # Name of this CA
  name: {{.CA.Name}}
  # Key file (is only used to import a private key into BCCSP)
  keyfile: /var/hyperledger/fabric-ca/msp-secret/keyfile
  # Certificate file (default: ca-cert.pem)
  certfile: /var/hyperledger/fabric-ca/msp-secret/certfile
  # Chain file
  chainfile:

metrics:
  provider: {{.Metrics.Provider}}
  statsd:
    network: {{.Metrics.Statsd.Network}}
    address: {{.Metrics.Statsd.Address}}
    writeInterval: {{.Metrics.Statsd.WriteInterval}}
    prefix: {{.Metrics.Statsd.Prefix}}

#############################################################################
#  The gencrl REST endpoint is used to generate a CRL that contains revoked
#  certificates. This section contains configuration options that are used
#  during gencrl request processing.
#############################################################################
crl:
  expiry: {{.CA.CRL.Expiry}}

#############################################################################
#  The registry section controls how the fabric-ca-server does two things:
#  1) authenticates enrollment requests which contain a username and password
#     (also known as an enrollment ID and secret).
#  2) once authenticated, retrieves the identity's attribute names and
#     values which the fabric-ca-server optionally puts into TCerts
#     which it issues for transacting on the Hyperledger Fabric blockchain.
#     These attributes are useful for making access control decisions in
#     chaincode.
#  There are two main configuration options:
#  1) The fabric-ca-server is the registry.
#     This is true if "ldap.enabled" in the ldap section below is false.
#  2) An LDAP server is the registry, in which case the fabric-ca-server
#     calls the LDAP server to perform these tasks.
#     This is true if "ldap.enabled" in the ldap section below is true,
#     which means this "registry" section is ignored.
#############################################################################
registry:
  maxenrollments: {{.CA.Registry.MaxEnrollments}}
  identities:
{{range .CA.Registry.Identities}}    - name: {{.Name}}
      pass: {{.Pass}}
      type: {{.Type}}
      affiliation: {{.Affiliation}}
      attrs:
        hf.Registrar.Roles: "{{.Attrs.RegistrarRoles}}"
        hf.Registrar.DelegateRoles: "{{.Attrs.DelegateRoles}}"
        hf.Registrar.Attributes: "{{.Attrs.Attributes}}"
        hf.Revoker: "{{.Attrs.Revoker}}"
        hf.IntermediateCA: "{{.Attrs.IntermediateCA}}"
        hf.GenCRL: "{{.Attrs.GenCRL}}"
        hf.AffiliationMgr: "{{.Attrs.AffiliationMgr}}"
{{end}}
#############################################################################
#  Database section
#  Supported types are: "sqlite3", "postgres", and "mysql".
#  The datasource value depends on the type.
#  If the type is "sqlite3", the datasource value is a file name to use
#  as the database store.  Since "sqlite3" is an embedded database, it
#  may not be used if you want to run the fabric-ca-server in a cluster.
#  To run the fabric-ca-server in a cluster, you must choose "postgres"
#  or "mysql".
#############################################################################
db:
  type: {{.Database.Type}}
  datasource: {{.Database.Datasource}}
  tls:
      enabled: false
      certfiles:
      client:
        certfile:
        keyfile:

#############################################################################
#  LDAP section
#  If LDAP is enabled, the fabric-ca-server calls LDAP to:
#  1) authenticate enrollment ID and secret (i.e. username and password)
#     for enrollment requests;
#  2) To retrieve identity attributes
#############################################################################
ldap:
   # Enables or disables the LDAP client (default: false)
   # If this is set to true, the "registry" section is ignored.
   enabled: false
   # The URL of the LDAP server
   url: ldap://<adminDN>:<adminPassword>@<host>:<port>/<base>
   # TLS configuration for the client connection to the LDAP server
   tls:
      certfiles:
      client:
         certfile:
         keyfile:
   # Attribute related configuration for mapping from LDAP entries to Fabric CA attributes
   attribute:
      # 'names' is an array of strings containing the LDAP attribute names which are
      # requested from the LDAP server for an LDAP identity's entry
      names: ['uid','member']
      # The 'converters' section is used to convert an LDAP entry to the value of
      # a fabric CA attribute.
      # For example, the following converts an LDAP 'uid' attribute
      # whose value begins with 'revoker' to a fabric CA attribute
      # named "hf.Revoker" with a value of "true" (because the boolean expression
      # evaluates to true).
      #    converters:
      #       - name: hf.Revoker
      #         value: attr("uid") =~ "revoker*"
      converters:
         - name:
           value:
      # The 'maps' section contains named maps which may be referenced by the 'map'
      # function in the 'converters' section to map LDAP responses to arbitrary values.
      # For example, assume a user has an LDAP attribute named 'member' which has multiple
      # values which are each a distinguished name (i.e. a DN). For simplicity, assume the
      # values of the 'member' attribute are 'dn1', 'dn2', and 'dn3'.
      # Further assume the following configuration.
      #    converters:
      #       - name: hf.Registrar.Roles
      #         value: map(attr("member"),"groups")
      #    maps:
      #       groups:
      #          - name: dn1
      #            value: peer
      #          - name: dn2
      #            value: client
      # The value of the user's 'hf.Registrar.Roles' attribute is then computed to be
      # "peer,client,dn3".  This is because the value of 'attr("member")' is
      # "dn1,dn2,dn3", and the call to 'map' with a 2nd argument of
      # "group" replaces "dn1" with "peer" and "dn2" with "client".
      maps:
         groups:
            - name:
              value:

#############################################################################
# Affiliations section, specified as hierarchical maps.
# Note: Affiliations are case sensitive except for the non-leaf affiliations.
#############################################################################
affiliations:
{{range .CA.Affiliations}}  {{.Name}}:
{{range .Departments}}    {{.}}:
{{end}}{{end}}

#############################################################################
#  Signing section
#
#  The "default" subsection is used to sign enrollment certificates;
#  the default expiration ("expiry" field) is "8760h", which is 1 year in hours.
#
#  The "ca" profile subsection is used to sign intermediate CA certificates;
#  the default expiration ("expiry" field) is "43800h" which is 5 years in hours.
#  Note that "isca" is true, meaning that it issues a CA certificate.
#  A maxpathlen of 0 means that the intermediate CA cannot issue other
#  intermediate CA certificates, though it can still issue end entity certificates.
#  (See RFC 5280, section 4.2.1.9)
#
#  The "tls" profile subsection is used to sign TLS certificate requests;
#  the default expiration ("expiry" field) is "8760h", which is 1 year in hours.
#############################################################################
signing:
  default:
    expiry: {{.CA.Signing.Default.Expiry}}
  profiles:
    ca:
      usage:
        - digital signature
        - key encipherment
        - cert sign
        - crl sign
      expiry: {{.CA.Signing.Profiles.CA.Expiry}}
      caconstraint:
        isca: true
        maxpathlen: {{.CA.Signing.Profiles.CA.CAConstraint.MaxPathLen}}
    tls:
      usage:
        - signing
        - key encipherment
        - server auth
        - client auth
      expiry: {{.CA.Signing.Profiles.TLS.Expiry}}

###########################################################################
#  Certificate Signing Request (CSR) section.
#  This controls the creation of the root CA certificate.
#  The expiration for the root CA certificate is configured with the
#  "ca.expiry" field below, whose default value is "131400h" which is
#  15 years in hours.
#  The pathlength field is used to limit CA certificate hierarchy as described
#  in section 4.2.1.9 of RFC 5280.
#  Examples:
#  1) No pathlength value means no limit is requested.
#  2) pathlength == 1 means a limit of 1 is requested which is the default for
#     a root CA.  This means the root CA can issue intermediate CA certificates,
#     but these intermediate CAs may not in turn issue other CA certificates
#     though they can still issue end entity certificates.
#  3) pathlength == 0 means a limit of 0 is requested;
#     this is the default for an intermediate CA, which means it can not issue
#     CA certificates though it can still issue end entity certificates.
###########################################################################
csr:
  cn: {{.CA.CSR.CN}}
  hosts:
{{range .CA.CSR.Hosts}}    - {{.}}
{{end}}  names:
{{range .CA.CSR.Names}}    - C: {{.C}}
      ST: {{.ST}}
      L: {{.L}}
      O: {{.O}}
      OU: {{.OU}}
{{end}}  ca:
    expiry: {{.CA.CSR.CA.Expiry}}
    pathlength: {{.CA.CSR.CA.PathLength}}

#############################################################################
# BCCSP (BlockChain Crypto Service Provider) section is used to select which
# crypto library implementation to use
#############################################################################
bccsp:
  default: {{.CA.BCCSP.Default}}
  sw:
    hash: {{.CA.BCCSP.SW.Hash}}
    security: {{.CA.BCCSP.SW.Security}}

#############################################################################
# Multi CA section (unused in a K8S deployment)
#############################################################################
cacount:
cafiles:
- /etc/hyperledger/fabric-ca-server/fabric-ca-server-config-tls.yaml

#############################################################################
# Intermediate CA section
#############################################################################
intermediate:
  parentserver:
    url: {{.CA.Intermediate.ParentServer.URL}}
    caname: {{.CA.Intermediate.ParentServer.CAName}}

#############################################################################
# Extra configuration options
# .e.g to enable adding and removing affiliations or identities
#############################################################################
cfg:
  identities:
    allowremove: {{.CA.CFG.Identities.AllowRemove}}
  affiliations:
    allowremove: {{.CA.CFG.Affiliations.AllowRemove}}

operations:
  # host and port for the operations server
  listenAddress: 0.0.0.0:9443

  # TLS configuration for the operations endpoint
  tls:
    # TLS enabled
    enabled: false

    # path to PEM encoded server certificate for the operations server
    cert:
      file: tls/server.crt

    # path to PEM encoded server key for the operations server
    key:
      file: tls/server.key

    # require client certificate authentication to access all resources
    clientAuthRequired: false

    # paths to PEM encoded ca certificates to trust for client authentication
    clientRootCAs:
      files: []
`

// TLSConfigTemplate is the Go template for the TLS CA configuration
const TLSConfigTemplate = `#############################################################################
#   This is a configuration file for the TLS CA server.
#############################################################################
version: "1.4.9"
# Server's listening port
port: 7054
# Enables debug logging
debug: {{.Debug}}
# Size limit of an acceptable CRL in bytes (default: 512000)
crlsizelimit: {{.CLRSizeLimit}}
#############################################################################
#  TLS section
#############################################################################
tls:
  # Enable TLS
  enabled: true
  # TLS for the server's listening port
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  clientauth:
    # Supported types: NoClientCert, RequestClientCert, RequireAnyClientCert, VerifyClientCertIfGiven and RequireAndVerifyClientCert.
    type: noclientcert
    enabled: true
    # List of root certificate authorities used when verifying client certificates
    certfiles:
#############################################################################
#  The CA section contains information related to the Certificate Authority
#  including the name of the CA, which should be unique for all members
#  of a blockchain network.  It also includes the key and certificate files
#  used when issuing enrollment certificates (ECerts) and transaction
#  certificates (TCerts).
#  The chainfile (if it exists) contains the certificate chain which
#  should be trusted for this CA, where the 1st in the chain is always the
#  root CA certificate.
#############################################################################
ca:
  # Name of this CA
  name: {{.TLSCA.Name}}
  # Key file (is only used to import a private key into BCCSP)
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  # Certificate file (default: ca-cert.pem)
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
  # Chain file
  chainfile:

metrics:
  provider: {{.Metrics.Provider}}
  statsd:
    network: {{.Metrics.Statsd.Network}}
    address: {{.Metrics.Statsd.Address}}
    writeInterval: {{.Metrics.Statsd.WriteInterval}}
    prefix: {{.Metrics.Statsd.Prefix}}

#############################################################################
#  The gencrl REST endpoint is used to generate a CRL that contains revoked
#  certificates. This section contains configuration options that are used
#  during gencrl request processing.
#############################################################################
crl:
  expiry: {{.TLSCA.CRL.Expiry}}

#############################################################################
#  The registry section controls how the fabric-ca-server does two things:
#  1) authenticates enrollment requests which contain a username and password
#     (also known as an enrollment ID and secret).
#  2) once authenticated, retrieves the identity's attribute names and
#     values which the fabric-ca-server optionally puts into TCerts
#     which it issues for transacting on the Hyperledger Fabric blockchain.
#     These attributes are useful for making access control decisions in
#     chaincode.
#  There are two main configuration options:
#  1) The fabric-ca-server is the registry.
#     This is true if "ldap.enabled" in the ldap section below is false.
#  2) An LDAP server is the registry, in which case the fabric-ca-server
#     calls the LDAP server to perform these tasks.
#     This is true if "ldap.enabled" in the ldap section below is true,
#     which means this "registry" section is ignored.
#############################################################################
registry:
  maxenrollments: {{.TLSCA.Registry.MaxEnrollments}}
  identities:
{{range .TLSCA.Registry.Identities}}    - name: {{.Name}}
      pass: {{.Pass}}
      type: {{.Type}}
      affiliation: {{.Affiliation}}
      attrs:
        hf.Registrar.Roles: "{{.Attrs.RegistrarRoles}}"
        hf.Registrar.DelegateRoles: "{{.Attrs.DelegateRoles}}"
        hf.Registrar.Attributes: "{{.Attrs.Attributes}}"
        hf.Revoker: "{{.Attrs.Revoker}}"
        hf.IntermediateCA: "{{.Attrs.IntermediateCA}}"
        hf.GenCRL: "{{.Attrs.GenCRL}}"
        hf.AffiliationMgr: "{{.Attrs.AffiliationMgr}}"
{{end}}
#############################################################################
#  Database section
#  Supported types are: "sqlite3", "postgres", and "mysql".
#  The datasource value depends on the type.
#  If the type is "sqlite3", the datasource value is a file name to use
#  as the database store.  Since "sqlite3" is an embedded database, it
#  may not be used if you want to run the fabric-ca-server in a cluster.
#  To run the fabric-ca-server in a cluster, you must choose "postgres"
#  or "mysql".
#############################################################################
db:
  type: {{.Database.Type}}
  datasource: {{.Database.Datasource}}
  tls:
      enabled: false
      certfiles:
      client:
        certfile:
        keyfile:

#############################################################################
#  LDAP section
#  If LDAP is enabled, the fabric-ca-server calls LDAP to:
#  1) authenticate enrollment ID and secret (i.e. username and password)
#     for enrollment requests;
#  2) To retrieve identity attributes
#############################################################################
ldap:
   # Enables or disables the LDAP client (default: false)
   # If this is set to true, the "registry" section is ignored.
   enabled: false
   # The URL of the LDAP server
   url: ldap://<adminDN>:<adminPassword>@<host>:<port>/<base>
   # TLS configuration for the client connection to the LDAP server
   tls:
      certfiles:
      client:
         certfile:
         keyfile:
   # Attribute related configuration for mapping from LDAP entries to Fabric CA attributes
   attribute:
      # 'names' is an array of strings containing the LDAP attribute names which are
      # requested from the LDAP server for an LDAP identity's entry
      names: ['uid','member']
      # The 'converters' section is used to convert an LDAP entry to the value of
      # a fabric CA attribute.
      # For example, the following converts an LDAP 'uid' attribute
      # whose value begins with 'revoker' to a fabric CA attribute
      # named "hf.Revoker" with a value of "true" (because the boolean expression
      # evaluates to true).
      #    converters:
      #       - name: hf.Revoker
      #         value: attr("uid") =~ "revoker*"
      converters:
         - name:
           value:
      # The 'maps' section contains named maps which may be referenced by the 'map'
      # function in the 'converters' section to map LDAP responses to arbitrary values.
      # For example, assume a user has an LDAP attribute named 'member' which has multiple
      # values which are each a distinguished name (i.e. a DN). For simplicity, assume the
      # values of the 'member' attribute are 'dn1', 'dn2', and 'dn3'.
      # Further assume the following configuration.
      #    converters:
      #       - name: hf.Registrar.Roles
      #         value: map(attr("member"),"groups")
      #    maps:
      #       groups:
      #          - name: dn1
      #            value: peer
      #          - name: dn2
      #            value: client
      # The value of the user's 'hf.Registrar.Roles' attribute is then computed to be
      # "peer,client,dn3".  This is because the value of 'attr("member")' is
      # "dn1,dn2,dn3", and the call to 'map' with a 2nd argument of
      # "group" replaces "dn1" with "peer" and "dn2" with "client".
      maps:
         groups:
            - name:
              value:

#############################################################################
# Affiliations section, specified as hierarchical maps.
# Note: Affiliations are case sensitive except for the non-leaf affiliations.
#############################################################################
affiliations:
{{range .TLSCA.Affiliations}}  {{.Name}}:
{{range .Departments}}    {{.}}:
{{end}}{{end}}

#############################################################################
#  Signing section
#
#  The "default" subsection is used to sign enrollment certificates;
#  the default expiration ("expiry" field) is "8760h", which is 1 year in hours.
#
#  The "ca" profile subsection is used to sign intermediate CA certificates;
#  the default expiration ("expiry" field) is "43800h" which is 5 years in hours.
#  Note that "isca" is true, meaning that it issues a CA certificate.
#  A maxpathlen of 0 means that the intermediate CA cannot issue other
#  intermediate CA certificates, though it can still issue end entity certificates.
#  (See RFC 5280, section 4.2.1.9)
#
#  The "tls" profile subsection is used to sign TLS certificate requests;
#  the default expiration ("expiry" field) is "8760h", which is 1 year in hours.
#############################################################################
signing:
  default:
    expiry: {{.TLSCA.Signing.Default.Expiry}}
  profiles:
    ca:
      usage:
        - digital signature
        - key encipherment
        - cert sign
        - crl sign
      expiry: {{.TLSCA.Signing.Profiles.CA.Expiry}}
      caconstraint:
        isca: true
        maxpathlen: {{.TLSCA.Signing.Profiles.CA.CAConstraint.MaxPathLen}}
    tls:
      usage:
        - signing
        - key encipherment
        - server auth
        - client auth
      expiry: {{.TLSCA.Signing.Profiles.TLS.Expiry}}

###########################################################################
#  Certificate Signing Request (CSR) section.
#  This controls the creation of the root CA certificate.
#  The expiration for the root CA certificate is configured with the
#  "ca.expiry" field below, whose default value is "131400h" which is
#  15 years in hours.
#  The pathlength field is used to limit CA certificate hierarchy as described
#  in section 4.2.1.9 of RFC 5280.
#  Examples:
#  1) No pathlength value means no limit is requested.
#  2) pathlength == 1 means a limit of 1 is requested which is the default for
#     a root CA.  This means the root CA can issue intermediate CA certificates,
#     but these intermediate CAs may not in turn issue other CA certificates
#     though they can still issue end entity certificates.
#  3) pathlength == 0 means a limit of 0 is requested;
#     this is the default for an intermediate CA, which means it can not issue
#     CA certificates though it can still issue end entity certificates.
###########################################################################
csr:
  cn: {{.TLSCA.CSR.CN}}
  hosts:
{{range .TLSCA.CSR.Hosts}}    - {{.}}
{{end}}  names:
{{range .TLSCA.CSR.Names}}    - C: {{.C}}
      ST: {{.ST}}
      L: {{.L}}
      O: {{.O}}
      OU: {{.OU}}
{{end}}  ca:
    expiry: {{.TLSCA.CSR.CA.Expiry}}
    pathlength: {{.TLSCA.CSR.CA.PathLength}}

#############################################################################
# BCCSP (BlockChain Crypto Service Provider) section is used to select which
# crypto library implementation to use
#############################################################################
bccsp:
  default: {{.TLSCA.BCCSP.Default}}
  sw:
    hash: {{.TLSCA.BCCSP.SW.Hash}}
    security: {{.TLSCA.BCCSP.SW.Security}}

#############################################################################
# Multi CA section (unused in a K8S deployment)
#############################################################################
cacount:
cafiles:
- /etc/hyperledger/fabric-ca-server/fabric-ca-server-config-tls.yaml

#############################################################################
# Intermediate CA section
#############################################################################
intermediate:
  parentserver:
    url: {{.TLSCA.Intermediate.ParentServer.URL}}
    caname: {{.TLSCA.Intermediate.ParentServer.CAName}}

#############################################################################
# Extra configuration options
# .e.g to enable adding and removing affiliations or identities
#############################################################################
cfg:
  identities:
    allowremove: {{.TLSCA.CFG.Identities.AllowRemove}}
  affiliations:
    allowremove: {{.TLSCA.CFG.Affiliations.AllowRemove}}

operations:
  # host and port for the operations server
  listenAddress: 0.0.0.0:9443

  # TLS configuration for the operations endpoint
  tls:
    # TLS enabled
    enabled: false

    # path to PEM encoded server certificate for the operations server
    cert:
      file: tls/server.crt

    # path to PEM encoded server key for the operations server
    key:
      file: tls/server.key

    # require client certificate authentication to access all resources
    clientAuthRequired: false

    # paths to PEM encoded ca certificates to trust for client authentication
    clientRootCAs:
      files: []
`

// ConfigData holds the data for template rendering
type ConfigData struct {
	Debug        bool
	CLRSizeLimit int
	Database     struct {
		Type       string
		Datasource string
	}
	Metrics struct {
		Provider string
		Statsd   struct {
			Network       string
			Address       string
			WriteInterval string
			Prefix        string
		}
	}
	CA    interface{}
	TLSCA interface{}
}

// GenerateConfigFromTemplate generates configuration using Go templates
func GenerateConfigFromTemplate(templateStr string, data interface{}) (string, error) {
	tmpl, err := template.New("config").Parse(templateStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
