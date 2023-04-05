package auth

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"gotest.tools/assert"
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

	cred, err := newAKSCredential(context.TODO(), file.Name())
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

	if _, err := newAKSCredential(context.TODO(), fileName); err == nil {
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

	if _, err := newAKSCredential(context.TODO(), file.Name()); err == nil {
		t.Fatal("expected to fail with bad json")
	}
}

func TestSetAuthConfigWithAuthFile(t *testing.T) {
	authFile := `
{
			"clientId": "######-tuhn-41af-re3e0-######",
  	"clientSecret": "######-###-####-####-######",
   "subscriptionId": "######-###-####-####-######",
   "tenantId": "######-###-####-####-######"

}`
	file, err := ioutil.TempFile("", "aks_auth_test")
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(file.Name())

	if _, err := file.Write([]byte(authFile)); err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_ID", "")
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AZURE_AUTH_LOCATION", file.Name())
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AKS_CREDENTIAL_LOCATION", "")
	if err != nil {
		t.Error(err)
	}

	azConfig := Config{}
	err = azConfig.SetAuthConfig(context.TODO())
	if err != nil {
		t.Error(err)
	}
	assert.Check(t, azConfig.Authorizer != nil, "Authorizer should be nil")

}

func TestSetAuthConfigWithAKSCredFile(t *testing.T) {
	aksCred := `
{
    "cloud":"AzurePublicCloud",
    "tenantId": "######-86f1-41af-91ab-######",
    "subscriptionId": "#######-4444-5555-6666-########",
    "aadClientId": "",
    "aadClientSecret": "msi",
    "resourceGroup": "MC_vk-test-rg",
    "location": "westcentralus",
 			"vnetName": "myAKSVNet",
    "vnetResourceGroup": "vk-aci-test-12917",
    "userAssignedIdentityID": "######-tuhn-41af-re3e0-######"
}`
	file, err := ioutil.TempFile("", "aks_auth_test")
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(file.Name())

	if _, err := file.Write([]byte(aksCred)); err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_ID", "")
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AZURE_AUTH_LOCATION", "")
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AKS_CREDENTIAL_LOCATION", file.Name())
	if err != nil {
		t.Error(err)
	}

	azConfig := Config{}
	err = azConfig.SetAuthConfig(context.TODO())
	if err != nil {
		t.Error(err)
	}
	assert.Check(t, azConfig.Authorizer != nil, "Authorizer should be nil")

}

func TestSetAuthConfigWithEnvVariablesOnly(t *testing.T) {
	err := os.Setenv("AZURE_AUTH_LOCATION", "")
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AKS_CREDENTIAL_LOCATION", "")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_SECRET", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("VIRTUALNODE_USER_IDENTITY_CLIENTID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_TENANT_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_SUBSCRIPTION_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	azConfig := Config{}
	err = azConfig.SetAuthConfig(context.TODO())
	if err != nil {
		t.Error(err)
	}
	assert.Check(t, azConfig.Authorizer != nil, "Authorizer should be nil")
}

func TestDecode(t *testing.T) {
	testCases := []struct {
		desc           string
		input          []byte
		expectedOutput string
	}{
		{
			desc:           "Testing Decode for UTF16LittleIndian Encoding",
			input:          []byte("\xFF\xFE\x68\x00\x65\x00\x6C\x00\x6C\x00\x6F\x00"),
			expectedOutput: string([]byte("\x68\x65\x6C\x6C\x6F")),
		},
		{
			desc:           "Testing Decode for UTF16BigIndian Encoding",
			input:          []byte("\xFE\xFF\x00\x68\x00\x65\x00\x6C\x00\x6C\x00\x6F"),
			expectedOutput: string([]byte("\x68\x65\x6C\x6C\x6F")),
		},
		{
			desc:           "Testing Decode for Unknown Encoding",
			input:          []byte("hello"),
			expectedOutput: string([]byte("hello")),
		},
	}

	authentication := Authentication{}

	for _, tc := range testCases {
		decodedValue, _ := authentication.decode(tc.input)
		assert.Equal(t, tc.expectedOutput, string(decodedValue), tc.desc)
	}
}

func TestGetMSICredential(t *testing.T) {

	err := os.Setenv("AZURE_AUTH_LOCATION", "")
	if err != nil {
		t.Error(err)
	}
	err = os.Setenv("AKS_CREDENTIAL_LOCATION", "")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_CLIENT_SECRET", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("VIRTUALNODE_USER_IDENTITY_CLIENTID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_TENANT_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	err = os.Setenv("AZURE_SUBSCRIPTION_ID", "######-###-####-####-######")
	if err != nil {
		t.Error(err)
	}

	azConfig := Config{}
	err = azConfig.SetAuthConfig(context.TODO())
	if err != nil {
		t.Error(err)
	}

	cred, err := azConfig.GetMSICredential(context.TODO())

	assert.Check(t, (cred != nil && err == nil), "Credential should not be nil with no errors while fetching it")
}
