package ca

import (
	"context"
	"fmt"
	"io"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/helpers"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CreateOptions struct {
	Name           string
	Namespace      string
	Image          string
	Version        string
	StorageClass   string
	StorageSize    string
	DatabaseType   string
	DatabaseSource string
	ServiceType    string
	Hosts          []string
	EnrollID       string
	EnrollSecret   string
	Output         bool
}

func (o CreateOptions) Validate() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	if o.Namespace == "" {
		o.Namespace = "default"
	}
	return nil
}

type createCmd struct {
	out    io.Writer
	errOut io.Writer
	opts   CreateOptions
}

func (c *createCmd) validate() error {
	return c.opts.Validate()
}

func (c *createCmd) run(_ []string) error {
	// Add the scheme for our custom resources
	s := scheme.Scheme
	if err := v1alpha1.AddToScheme(s); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	// Create client using helper
	k8sClient, err := helpers.GetControllerRuntimeClient(s)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Create namespace if it doesn't exist
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.opts.Namespace,
		},
	}

	err = k8sClient.Get(context.Background(), client.ObjectKey{Name: c.opts.Namespace}, namespace)
	if err != nil {
		// Namespace doesn't exist, create it
		if err := k8sClient.Create(context.Background(), namespace); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
		log.Infof("Created namespace %s", c.opts.Namespace)
	}

	// Create CA resource
	ca := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.opts.Name,
			Namespace: c.opts.Namespace,
		},
		Spec: v1alpha1.CASpec{
			Image:   c.opts.Image,
			Version: c.opts.Version,
			Hosts:   c.opts.Hosts,
			Database: v1alpha1.FabricCADatabase{
				Type:       c.opts.DatabaseType,
				Datasource: c.opts.DatabaseSource,
			},
			Service: v1alpha1.FabricCASpecService{
				ServiceType: corev1.ServiceType(c.opts.ServiceType),
			},
			Storage: v1alpha1.FabricCAStorage{
				StorageClass: c.opts.StorageClass,
				AccessMode:   "ReadWriteOnce",
				Size:         c.opts.StorageSize,
			},
			CA: v1alpha1.FabricCAItemConf{
				Name: "ca",
				CSR: v1alpha1.FabricCACSR{
					CN:    "ca",
					Hosts: []string{"localhost"},
					Names: []v1alpha1.FabricCANames{
						{
							C:  "US",
							ST: "North Carolina",
							O:  "Hyperledger",
							L:  "Raleigh",
							OU: "Fabric",
						},
					},
					CA: v1alpha1.FabricCACSRCA{
						Expiry:     "131400h",
						PathLength: 0,
					},
				},
				CRL: v1alpha1.FabricCACRL{
					Expiry: "24h",
				},
				Registry: v1alpha1.FabricCAItemRegistry{
					MaxEnrollments: -1,
					Identities: []v1alpha1.FabricCAIdentity{
						{
							Name:        c.opts.EnrollID,
							Pass:        c.opts.EnrollSecret,
							Type:        "client",
							Affiliation: "",
							Attrs: v1alpha1.FabricCAIdentityAttrs{
								RegistrarRoles: "*",
								DelegateRoles:  "*",
								Attributes:     "*",
								Revoker:        true,
								IntermediateCA: true,
								GenCRL:         true,
								AffiliationMgr: true,
							},
						},
					},
				},
				BCCSP: v1alpha1.FabricCAItemBCCSP{
					Default: "SW",
					SW: v1alpha1.FabricCAItemBCCSPSW{
						Hash:     "SHA2",
						Security: 256,
					},
				},
			},
			TLSCA: v1alpha1.FabricCAItemConf{
				Name: "tlsca",
				CSR: v1alpha1.FabricCACSR{
					CN:    "tlsca",
					Hosts: []string{"localhost"},
					Names: []v1alpha1.FabricCANames{
						{
							C:  "US",
							ST: "North Carolina",
							O:  "Hyperledger",
							L:  "Raleigh",
							OU: "Fabric",
						},
					},
					CA: v1alpha1.FabricCACSRCA{
						Expiry:     "131400h",
						PathLength: 0,
					},
				},
				CRL: v1alpha1.FabricCACRL{
					Expiry: "24h",
				},
				Registry: v1alpha1.FabricCAItemRegistry{
					MaxEnrollments: -1,
					Identities: []v1alpha1.FabricCAIdentity{
						{
							Name:        c.opts.EnrollID,
							Pass:        c.opts.EnrollSecret,
							Type:        "client",
							Affiliation: "",
							Attrs: v1alpha1.FabricCAIdentityAttrs{
								RegistrarRoles: "*",
								DelegateRoles:  "*",
								Attributes:     "*",
								Revoker:        true,
								IntermediateCA: true,
								GenCRL:         true,
								AffiliationMgr: true,
							},
						},
					},
				},
				BCCSP: v1alpha1.FabricCAItemBCCSP{
					Default: "SW",
					SW: v1alpha1.FabricCAItemBCCSPSW{
						Hash:     "SHA2",
						Security: 256,
					},
				},
			},
			TLS: v1alpha1.FabricCATLS{
				Subject: v1alpha1.FabricCANames{
					C:  "US",
					ST: "North Carolina",
					O:  "Hyperledger",
					L:  "Raleigh",
					OU: "Fabric",
				},
			},
			CredentialStore: v1alpha1.CredentialStoreKubernetes,
		},
	}

	if c.opts.Output {
		// Output YAML instead of creating
		caYaml, err := helpers.ToYaml([]runtime.Object{ca})
		if err != nil {
			return fmt.Errorf("failed to marshal CA to YAML: %w", err)
		}
		fmt.Fprintf(c.out, "%s\n", caYaml[0])
		return nil
	}

	// Create the CA using the Kubernetes client
	if err := k8sClient.Create(context.Background(), ca); err != nil {
		return fmt.Errorf("failed to create CA: %w", err)
	}

	log.Infof("CA %s created successfully in namespace %s", c.opts.Name, c.opts.Namespace)
	return nil
}

func newCreateCACmd(out io.Writer, errOut io.Writer) *cobra.Command {
	c := &createCmd{
		out:    out,
		errOut: errOut,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new Fabric CA",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.validate(); err != nil {
				return err
			}
			return c.run(args)
		},
	}

	cmd.Flags().StringVar(&c.opts.Name, "name", "", "Name of the CA")
	cmd.Flags().StringVar(&c.opts.Namespace, "namespace", "default", "Namespace for the CA")
	cmd.Flags().StringVar(&c.opts.Image, "image", "hyperledger/fabric-ca:1.4.3", "CA image")
	cmd.Flags().StringVar(&c.opts.Version, "version", "1.4.3", "CA version")
	cmd.Flags().StringVar(&c.opts.StorageClass, "storage-class", "", "Storage class for PVC")
	cmd.Flags().StringVar(&c.opts.StorageSize, "storage-size", "1Gi", "Storage size for PVC")
	cmd.Flags().StringVar(&c.opts.DatabaseType, "db-type", "sqlite3", "Database type")
	cmd.Flags().StringVar(&c.opts.DatabaseSource, "db-source", "/var/hyperledger/fabric-ca/fabric-ca-server.db", "Database source")
	cmd.Flags().StringVar(&c.opts.ServiceType, "service-type", "ClusterIP", "Service type")
	cmd.Flags().StringSliceVar(&c.opts.Hosts, "hosts", []string{}, "Host names for the CA")
	cmd.Flags().StringVar(&c.opts.EnrollID, "enroll-id", "admin", "Enrollment ID")
	cmd.Flags().StringVar(&c.opts.EnrollSecret, "enroll-secret", "adminpw", "Enrollment secret")
	cmd.Flags().BoolVar(&c.opts.Output, "output", false, "Output YAML instead of applying")

	cmd.MarkFlagRequired("name")

	return cmd
}
