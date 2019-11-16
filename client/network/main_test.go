package network

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	azure "github.com/virtual-kubelet/azure-aci/client"
	"github.com/virtual-kubelet/azure-aci/client/resourcegroups"
	"github.com/virtual-kubelet/azure-aci/test/config"
)

var (
	testAuth      *azure.Authentication
	resourceGroup string
	location      string
)

func TestMain(m *testing.M) {
	cfg := config.FromEnv()
	uid := uuid.New()
	cfg.ResourceGroup += "-" + uid.String()[0:6]
	resourceGroup = cfg.ResourceGroup
	location = cfg.Location

	if err := setupAuth(); err != nil {
		fmt.Fprintln(os.Stderr, "Error setting up auth:", err)
		os.Exit(1)
	}

	c, err := resourcegroups.NewClient(testAuth, "unit-test")
	if err != nil {
		os.Exit(1)
	}
	_, err = c.CreateResourceGroup(cfg.ResourceGroup, resourcegroups.Group{
		Name:     cfg.ResourceGroup,
		Location: cfg.Location,
	})
	if err != nil {
		os.Exit(1)
	}

	code := m.Run()

	if err := c.DeleteResourceGroup(cfg.ResourceGroup); err != nil {
		fmt.Fprintln(os.Stderr, "error removing resource group:", err)
	}

	os.Exit(code)
}

var authOnce sync.Once

func setupAuth() error {
	var err error
	authOnce.Do(func() {
		testAuth, err = azure.NewAuthenticationFromFile(os.Getenv("AZURE_AUTH_LOCATION"))
		if err != nil {
			testAuth, err = azure.NewAuthenticationFromFile(os.Getenv("AZURE_AUTH_LOCATION"))
		}
		if err != nil {
			err = errors.Wrap(err, "failed to load Azure authentication file")
		}
	})
	return err
}

func newTestClient(t *testing.T) *Client {
	if err := setupAuth(); err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(testAuth, "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
