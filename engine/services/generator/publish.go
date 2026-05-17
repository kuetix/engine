package generator

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/kuetix/logger"
)

// SignPackage signs a package file with the provided API token
func SignPackage(packageFile, apiToken string) error {
	// Read the package file
	data, err := os.ReadFile(packageFile)
	if err != nil {
		return fmt.Errorf("failed to read package file: %w", err)
	}

	// Parse the package
	var pkg PackageInfo
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package file: %w", err)
	}

	// Generate signature using HMAC-SHA256
	signature := generateSignature(data, apiToken)
	pkg.Signature = signature

	// Write back to file
	signedData, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal signed package: %w", err)
	}

	if err := os.WriteFile(packageFile, signedData, 0644); err != nil {
		return fmt.Errorf("failed to write signed package: %w", err)
	}

	logger.Infof("Package signed with signature: %s", signature[:16]+"...")
	return nil
}

// generateSignature creates a HMAC-SHA256 signature
func generateSignature(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// UploadPackage uploads a package to the server
//
//goland:noinspection GoUnusedExportedFunction
func UploadPackage(packageFile, apiToken, serverURL string) error {
	// Read the package file
	data, err := os.ReadFile(packageFile)
	if err != nil {
		return fmt.Errorf("failed to read package file: %w", err)
	}

	// Parse the package
	var pkg PackageInfo
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package file: %w", err)
	}

	// Determine server URL
	if serverURL == "" {
		return fmt.Errorf("server URL is required")
	}

	endpoint := fmt.Sprintf("%s/v1/packages/upload", serverURL)

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Set("X-Package-Name", pkg.Name)
	req.Header.Set("X-Package-Version", pkg.Version)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			logger.Errorf("Failed to close response body: %s", err)
		}
	}(resp.Body)

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	logger.Infof("Upload response: %s", string(respBody))
	return nil
}

// PublishPackage publishes a package version on the server
//
//goland:noinspection GoUnusedExportedFunction
func PublishPackage(packageFile, apiToken, serverURL string, public bool, price string) error {
	// Read the package file
	data, err := os.ReadFile(packageFile)
	if err != nil {
		return fmt.Errorf("failed to read package file: %w", err)
	}

	// Parse the package
	var pkg PackageInfo
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package file: %w", err)
	}

	// Determine server URL
	if serverURL == "" {
		return fmt.Errorf("server URL is required")
	}

	endpoint := fmt.Sprintf("%s/v1/packages/%s/versions/%s/publish", serverURL, pkg.Name, pkg.Version)

	// Create publish request payload
	publishData := map[string]interface{}{
		"package":    pkg,
		"visibility": "private",
		"price":      price,
	}

	if public {
		publishData["visibility"] = "public"
	}

	payloadData, err := json.Marshal(publishData)
	if err != nil {
		return fmt.Errorf("failed to marshal publish data: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(payloadData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			logger.Errorf("Failed to close response body: %s", err)
		}
	}(resp.Body)

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	visibility := "private"
	if public {
		visibility = "public"
	}
	logger.Infof("Package published as %s. Response: %s", visibility, string(respBody))
	return nil
}
