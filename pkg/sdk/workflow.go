package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

const (
	uploadEndpoint = "/upload"
	uploadFileKey  = "file"
)

func (sdk *propSDK) UploadTaskFile(id, filePath string) (Task, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Task{}, err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(uploadFileKey, filepath.Base(filePath))
	if err != nil {
		return Task{}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return Task{}, err
	}
	if err := writer.Close(); err != nil {
		return Task{}, err
	}

	reqURL := sdk.managerURL + tasksEndpoint + "/" + id + uploadEndpoint

	req, err := http.NewRequest(http.MethodPut, reqURL, body)
	if err != nil {
		return Task{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := sdk.client.Do(req)
	if err != nil {
		return Task{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Task{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return Task{}, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	var t Task
	if err := json.Unmarshal(respBody, &t); err != nil {
		return Task{}, err
	}

	return t, nil
}

func (sdk *propSDK) CreateWorkflow(tasks []Task) ([]Task, error) {
	data, err := json.Marshal(struct {
		Tasks []Task `json:"tasks"`
	}{Tasks: tasks})
	if err != nil {
		return nil, err
	}

	reqURL := sdk.managerURL + "/workflows"

	body, err := sdk.processRequest(http.MethodPost, reqURL, data, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tasks []Task `json:"tasks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return resp.Tasks, nil
}
