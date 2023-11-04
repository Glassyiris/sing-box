package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	URL "net/url"
)

type FileVehicle struct {
	path string
}

func (f *FileVehicle) Type() VehicleType {
	return File
}

func (f *FileVehicle) Path() string {
	return f.path
}

func (f *FileVehicle) Read() ([]byte, error) {
	return os.ReadFile(f.path)
}

func NewFileVehicle(path string) *FileVehicle {
	return &FileVehicle{path: path}
}

type HTTPVehicle struct {
	url  string
	path string
}

func (h *HTTPVehicle) Url() string {
	return h.url
}

func (h *HTTPVehicle) Type() VehicleType {
	return HTTP
}

func (h *HTTPVehicle) Path() string {
	return h.path
}

func (h *HTTPVehicle) Read() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	resp, err := HttpRequest(ctx, h.url, http.MethodGet, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errors.New(resp.Status)
	}
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func NewHTTPVehicle(url string, path string) *HTTPVehicle {
	return &HTTPVehicle{url, path}
}

func HttpRequest(ctx context.Context, url, method string, header map[string][]string, body io.Reader) (*http.Response, error) {
	method = strings.ToUpper(method)
	urlRes, err := URL.Parse(url)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, urlRes.String(), body)
	for k, v := range header {
		for _, v := range v {
			req.Header.Add(k, v)
		}
	}

	if err != nil {
		return nil, err
	}

	if user := urlRes.User; user != nil {
		password, _ := user.Password()
		req.SetBasicAuth(user.Username(), password)
	}

	req = req.WithContext(ctx)

	transport := &http.Transport{
		// from http.DefaultTransport
		DisableKeepAlives:     runtime.GOOS == "android",
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// TODO: need  impl detour logic
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// if conn, err := inner.HandleTcp(address); err == nil {
			// 	return conn, nil
			// } else {
			d := net.Dialer{}
			return d.DialContext(ctx, network, address)
			//}
		},
	}

	client := http.Client{Transport: transport}
	return client.Do(req)
}
