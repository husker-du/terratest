// Functions defined in main.go represent the "interface" for this test tool.
// They generally handle all setup, test execution, and teardown.
// Consumers of this library should only use other functions if you need to deviate from the setup/teardown in these "main" functions.
package main

import (
	"fmt"

	"github.com/gruntwork-io/terraform-test/log"
	"github.com/gruntwork-io/terraform-test/aws"
	"github.com/gruntwork-io/terraform-test/util"
	"github.com/gruntwork-io/terraform-test/terraform"
)

func main() {
}

// This function wraps all setup and teardown required for a Terraform Apply operation
func TerraformApply(testName string, templatePath string, vars map[string]string, attemptTerraformRetry bool) error {
	logger := log.NewLogger(testName)

	// SETUP

	// Generate random values to allow tests to run in parallel
	// - Note that if two tests run at the same time we don't expect a conflict, but by randomly selecting a region
	//   we can reduce the likelihood of hitting standard AWS limits.
	// - The "id" is used to namespace all terraform resource names. In fact, Terraform templates should be written
	//   so that all resources that terraform creates have namespaced names to enable parallel test runs of the same test.
	region := aws.GetRandomRegion()
	id := util.UniqueId()
	logger.Printf("Random values selected. Region = %s, Id = %s\n", region, id)

	// Generate a random RSA Keypair and upload it to AWS to create a new EC2 Keypair.
	keyPair, err := util.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("Failed to generate keypair: %s\n", err.Error())
	}
	logger.Println("Generated keypair. Printing out private key...")
	logger.Printf("%s", keyPair.PrivateKey)

	logger.Println("Creating EC2 Keypair...")
	aws.CreateEC2KeyPair(region, id, keyPair.PublicKey)
	defer aws.DeleteEC2KeyPair(region, id)

	// Configure terraform to use Remote State.
	err = aws.AssertS3BucketExists(TF_REMOTE_STATE_S3_BUCKET_REGION, TF_REMOTE_STATE_S3_BUCKET_NAME)
	if err != nil {
		return fmt.Errorf("Test failed because the S3 Bucket '%s' does not exist in the '%s' region.\n", TF_REMOTE_STATE_S3_BUCKET_NAME, TF_REMOTE_STATE_S3_BUCKET_REGION)
	}

	terraform.ConfigureRemoteState(templatePath, TF_REMOTE_STATE_S3_BUCKET_NAME, id + "/main.tf", TF_REMOTE_STATE_S3_BUCKET_REGION, logger)

	// TEST

	// Apply the Terraform template
	logger.Println("Running terraform apply...")

	if attemptTerraformRetry {
		err = terraform.ApplyWithRetry(templatePath, vars, logger)
	} else {
		err = terraform.Apply(templatePath, vars, logger)
	}
	defer TerraformDestroyHelper(testName, templatePath, vars)
	if err != nil {
		return fmt.Errorf("Failed to terraform apply: %s\n", err.Error())
	}

	return nil
}

// Helper function that allows Terraform Destroy to be called after Terraform Apply returns
func TerraformDestroyHelper(testName string, templatePath string, vars map[string]string) {
	logger := log.NewLogger(testName)
	err := terraform.Destroy(templatePath, vars, logger)
	if err != nil {
		fmt.Printf(`Failed to terraform destroy.
		** WARNING ** Terraform destroy has failed which means you must manually delete all resources created by
		the terraform template at %s.  '%s' was used for the test name.

		Scroll up to see the AWS region.

		Official Error Message:\n
		%s\n`, templatePath, testName, err.Error())
	}
}