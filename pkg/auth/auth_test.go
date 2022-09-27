package auth

import (
	"io/ioutil"
	"os"
	"testing"
)

const cred = `
{
    "cloud":"AzurePublicCloud",
    "tenantId": "######-86f1-41af-91ab-######",
    "subscriptionId": "#######-4444-5555-6666-########",
    "aadClientId": "123",
    "aadClientSecret": "456",
    "resourceGroup": "vk-test-rg",
    "location": "westcentralus"
}`

func TestAKSCred(t *testing.T) {
	file, err := ioutil.TempFile("", "aks_auth_test")
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(file.Name())

	if _, err := file.Write([]byte(cred)); err != nil {
		t.Error(err)
	}

	cred, err := newAKSCredential(file.Name())
	if err != nil {
		t.Error(err)
	}
	wanted := "AzurePublicCloud"
	if cred.Cloud != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.Cloud)
	}

	wanted = "######-86f1-41af-91ab-######"
	if cred.TenantID != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.TenantID)
	}

	wanted = "#######-4444-5555-6666-########"
	if cred.SubscriptionID != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.SubscriptionID)
	}
	wanted = "123"
	if cred.ClientID != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.ClientID)
	}

	wanted = "456"
	if cred.ClientSecret != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.ClientSecret)
	}

	wanted = "vk-test-rg"
	if cred.ResourceGroup != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.ResourceGroup)
	}

	wanted = "westcentralus"
	if cred.Region != wanted {
		t.Errorf("Wanted %s, got %s.", wanted, cred.Region)
	}
}

func TestAKSCredFileNotFound(t *testing.T) {
	file, err := ioutil.TempFile("", "AKS_test")
	if err != nil {
		t.Error(err)
	}

	fileName := file.Name()

	if err := file.Close(); err != nil {
		t.Error(err)
	}

	os.Remove(fileName)

	if _, err := newAKSCredential(fileName); err == nil {
		t.Fatal("expected to fail with bad json")
	}
}

const credBad = `
{
    "cloud":"AzurePublicCloud",
    "tenantId": "######-86f1-41af-91ab-######",
    "subscriptionId": "#######-4444-5555-6666-########",
    "aadClientId": "123",
    "aadClientSecret": "456",
    "resourceGroup": "vk-test-rg",`

func TestAKSCredBadJson(t *testing.T) {
	file, err := ioutil.TempFile("", "aks_auth_test")
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(file.Name())

	if _, err := file.Write([]byte(credBad)); err != nil {
		t.Error(err)
	}

	if _, err := newAKSCredential(file.Name()); err == nil {
		t.Fatal("expected to fail with bad json")
	}
}
