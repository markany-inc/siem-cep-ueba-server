package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var Client = &http.Client{Timeout: 30 * time.Second}

type OSClient struct {
	BaseURL string
}

func NewOSClient(url string) *OSClient {
	return &OSClient{BaseURL: url}
}

func (c *OSClient) Search(index string, body interface{}) ([]map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.BaseURL+"/"+index+"/_search", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search failed: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	hitsObj, ok := result["hits"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	hitArr, ok := hitsObj["hits"].([]interface{})
	if !ok {
		return nil, nil
	}
	docs := make([]map[string]interface{}, 0, len(hitArr))
	for _, h := range hitArr {
		hit, _ := h.(map[string]interface{})
		if doc, ok := hit["_source"].(map[string]interface{}); ok {
			doc["_id"] = hit["_id"]
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func (c *OSClient) Put(index, docID string, doc interface{}) error {
	data, _ := json.Marshal(doc)
	url := fmt.Sprintf("%s/%s/_doc/%s", c.BaseURL, index, docID)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenSearch PUT failed: %s", body)
	}
	return nil
}

func (c *OSClient) Refresh(index string) {
	http.Post(c.BaseURL+"/"+index+"/_refresh", "application/json", nil)
}

func (c *OSClient) Delete(index, docID string) error {
	req, _ := http.NewRequest("DELETE", c.BaseURL+"/"+index+"/_doc/"+docID, nil)
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenSearch Delete failed: %s", body)
	}
	return nil
}

func (c *OSClient) Index(index string, doc interface{}) error {
	data, _ := json.Marshal(doc)
	resp, err := Client.Post(c.BaseURL+"/"+index+"/_doc", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenSearch Index failed: %s", body)
	}
	return nil
}

func (c *OSClient) Count(index string, query interface{}) (int, error) {
	data, _ := json.Marshal(map[string]interface{}{"query": query})
	resp, err := Client.Post(c.BaseURL+"/"+index+"/_count", "application/json", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if cnt, ok := result["count"].(float64); ok {
		return int(cnt), nil
	}
	return 0, nil
}

func (c *OSClient) SearchRaw(index string, body interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.BaseURL+"/"+index+"/_search", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *OSClient) GetMapping(index string) (map[string]interface{}, error) {
	resp, err := Client.Get(c.BaseURL + "/" + index + "/_mapping")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}
