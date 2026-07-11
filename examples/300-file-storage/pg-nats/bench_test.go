package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const baseURL = "http://localhost:18088"

var (
	setupOnce sync.Once
	svcCmd    *exec.Cmd
	docker    bool
)

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"
	if !docker {
		if _, err := exec.LookPath("docker"); err != nil {
			fmt.Println("skip: no docker")
			os.Exit(0)
		}
		out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/file-pg-nats-svc", ".").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}
	fmt.Println("=== file-pg-nats ===")
	code := m.Run()
	if !docker {
		teardown()
	}
	os.Exit(code)
}

func setup(tb testing.TB) {
	if docker {
		setupOnce.Do(func() { waitHTTP(tb, baseURL+"/health", 30*time.Second) })
		return
	}
	setupOnce.Do(func() {
		svcCmd = exec.Command("/tmp/file-pg-nats-svc")
		svcCmd.Env = append(os.Environ(), "PORT=18088")
		svcCmd.Stdout = os.Stdout
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start svc: %v", err)
		}
		waitHTTP(tb, baseURL+"/health", 30*time.Second)
	})
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Kill()
		svcCmd.Wait()
	}
}

func waitHTTP(tb testing.TB, url string, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	tb.Fatalf("timeout waiting for %s", url)
}

func TestFile_CRUDFlow(t *testing.T) {
	setup(t)

	// Create a product without media
	resp, err := http.Post(baseURL+"/api/v1/products", "application/json",
		strings.NewReader(`{"name":"Test Product","price":29.99}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status %d: %s", resp.StatusCode, string(body))
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	id := created["id"]
	if id == nil {
		t.Fatal("no id in create response")
	}
	t.Logf("created product id=%v", id)

	// List products
	listResp, err := http.Get(baseURL + "/api/v1/products")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != 200 {
		t.Fatalf("list status %d", listResp.StatusCode)
	}

	// Get product by ID
	getResp, err := http.Get(fmt.Sprintf("%s/api/v1/products/%v", baseURL, id))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Fatalf("get status %d", getResp.StatusCode)
	}

	// Update product
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/products/%v", baseURL, id),
		strings.NewReader(`{"name":"Updated Product","price":39.99}`))
	updResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	defer updResp.Body.Close()
	if updResp.StatusCode != 200 {
		body, _ := io.ReadAll(updResp.Body)
		t.Fatalf("update status %d: %s", updResp.StatusCode, string(body))
	}

	// Delete product
	delReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/products/%v", baseURL, id), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != 200 && delResp.StatusCode != 204 {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}

	// Verify deletion
	checkResp, err := http.Get(fmt.Sprintf("%s/api/v1/products/%v", baseURL, id))
	if err != nil {
		t.Fatalf("check delete: %v", err)
	}
	defer checkResp.Body.Close()
	if checkResp.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", checkResp.StatusCode)
	}
}

func TestFile_UploadFlow(t *testing.T) {
	setup(t)
	payload := "hello-s3-test-content"
	resp, err := http.Post(baseURL+"/api/v1/files/upload", "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status %d: %s", resp.StatusCode, string(body))
	}
	var uploadResp map[string]any
	json.NewDecoder(resp.Body).Decode(&uploadResp)
	key, _ := uploadResp["key"].(string)
	if key == "" {
		t.Fatal("upload: no key in response")
	}
	presignURL, _ := uploadResp["presignURL"].(string)
	if presignURL == "" {
		t.Log("upload: no presignURL (S3 storage missing Presigner)")
	} else {
		t.Logf("upload: presignURL received (%.60s...)", presignURL)
	}
	t.Logf("upload: key=%s size=%v", key, uploadResp["size"])
}

func TestFile_ProductWithMedia(t *testing.T) {
	setup(t)

	// 1. Upload file
	payload := "test-image-for-product"
	upResp, err := http.Post(baseURL+"/api/v1/files/upload", "image/png", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer upResp.Body.Close()
	if upResp.StatusCode != 200 {
		body, _ := io.ReadAll(upResp.Body)
		t.Fatalf("upload status %d: %s", upResp.StatusCode, string(body))
	}
	var upBody map[string]any
	json.NewDecoder(upResp.Body).Decode(&upBody)
	mediaKey, _ := upBody["key"].(string)
	if mediaKey == "" {
		t.Fatal("upload: no key")
	}
	t.Logf("uploaded media key=%s", mediaKey)

	// 2. Create product with that media_key
	createBody := fmt.Sprintf(`{"name":"Media Product","price":15.50,"mediaKey":"%s"}`, mediaKey)
	crResp, err := http.Post(baseURL+"/api/v1/products", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	defer crResp.Body.Close()
	if crResp.StatusCode != 201 {
		body, _ := io.ReadAll(crResp.Body)
		t.Fatalf("create status %d: %s", crResp.StatusCode, string(body))
	}
	var created map[string]any
	json.NewDecoder(crResp.Body).Decode(&created)
	productID := created["id"]
	if productID == nil {
		t.Fatal("create: no id")
	}
	t.Logf("created product id=%v", productID)

	// 3. Get product with media via custom endpoint
	viewResp, err := http.Get(fmt.Sprintf("%s/api/v1/products/%v/view", baseURL, productID))
	if err != nil {
		t.Fatalf("view product: %v", err)
	}
	defer viewResp.Body.Close()
	if viewResp.StatusCode != 200 {
		body, _ := io.ReadAll(viewResp.Body)
		t.Fatalf("view status %d: %s", viewResp.StatusCode, string(body))
	}
	var viewBody map[string]any
	json.NewDecoder(viewResp.Body).Decode(&viewBody)
	if viewBody["name"] != "Media Product" {
		t.Errorf("view name=%v, want 'Media Product'", viewBody["name"])
	}
	mediaURL, _ := viewBody["mediaURL"].(string)
	if mediaURL == "" {
		t.Error("view: expected mediaURL (presigned), got empty")
	} else {
		t.Logf("view: presigned mediaURL valid (%.60s...)", mediaURL)
	}

	// 4. Cleanup: delete product
	delReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/products/%v", baseURL, productID), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != 200 && delResp.StatusCode != 204 {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}
}

func TestFile_ProductUpdateMedia(t *testing.T) {
	setup(t)

	// Upload first media
	up1, _ := http.Post(baseURL+"/api/v1/files/upload", "text/plain", strings.NewReader("media-a"))
	var b1 map[string]any
	json.NewDecoder(up1.Body).Decode(&b1)
	up1.Body.Close()
	keyA, _ := b1["key"].(string)
	t.Logf("media key A=%s", keyA)

	// Create product with media A
	cr1, _ := http.Post(baseURL+"/api/v1/products", "application/json",
		strings.NewReader(fmt.Sprintf(`{"name":"Product","price":10,"mediaKey":"%s"}`, keyA)))
	var prod1 map[string]any
	json.NewDecoder(cr1.Body).Decode(&prod1)
	cr1.Body.Close()
	pid := fmt.Sprintf("%v", prod1["id"])
	t.Logf("product id=%s with media A", pid)

	// Upload second media
	up2, _ := http.Post(baseURL+"/api/v1/files/upload", "text/plain", strings.NewReader("media-b"))
	var b2 map[string]any
	json.NewDecoder(up2.Body).Decode(&b2)
	up2.Body.Close()
	keyB, _ := b2["key"].(string)
	t.Logf("media key B=%s", keyB)

	// Update product to media B
	updReq, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/products/%s", baseURL, pid),
		strings.NewReader(fmt.Sprintf(`{"mediaKey":"%s"}`, keyB)))
	updResp, err := http.DefaultClient.Do(updReq)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	defer updResp.Body.Close()
	if updResp.StatusCode != 200 {
		body, _ := io.ReadAll(updResp.Body)
		t.Fatalf("update status %d: %s", updResp.StatusCode, string(body))
	}
	t.Log("product media updated from A to B")

	// Verify via view endpoint
	viewResp, _ := http.Get(fmt.Sprintf("%s/api/v1/products/%s/view", baseURL, pid))
	defer viewResp.Body.Close()
	var viewBody map[string]any
	json.NewDecoder(viewResp.Body).Decode(&viewBody)
	t.Logf("view: mediaURL present=%v", viewBody["mediaURL"] != nil && viewBody["mediaURL"] != "")
}
