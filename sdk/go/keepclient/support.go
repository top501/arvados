package keepclient

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"

	"git.curoverse.com/arvados.git/sdk/go/arvadosclient"
	"git.curoverse.com/arvados.git/sdk/go/streamer"
)

// Function used to emit debug messages. The easiest way to enable
// keepclient debug messages in your application is to assign
// log.Printf to DebugPrintf.
var DebugPrintf = func(string, ...interface{}) {}

func init() {
	if arvadosclient.StringBool(os.Getenv("ARVADOS_DEBUG")) {
		DebugPrintf = log.Printf
	}
}

type keepService struct {
	Uuid     string `json:"uuid"`
	Hostname string `json:"service_host"`
	Port     int    `json:"service_port"`
	SSL      bool   `json:"service_ssl_flag"`
	SvcType  string `json:"service_type"`
	ReadOnly bool   `json:"read_only"`
}

// Md5String returns md5 hash for the bytes in the given string
func Md5String(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}

type svcList struct {
	Items []keepService `json:"items"`
}

type uploadStatus struct {
	err             error
	url             string
	statusCode      int
	replicas_stored int
	response        string
}

func (this *KeepClient) uploadToKeepServer(host string, hash string, body io.ReadCloser,
	upload_status chan<- uploadStatus, expectedLength int64, requestID int32) {

	var req *http.Request
	var err error
	var url = fmt.Sprintf("%s/%s", host, hash)
	if req, err = http.NewRequest("PUT", url, nil); err != nil {
		DebugPrintf("DEBUG: [%08x] Error creating request PUT %v error: %v", requestID, url, err.Error())
		upload_status <- uploadStatus{err, url, 0, 0, ""}
		body.Close()
		return
	}

	req.ContentLength = expectedLength
	if expectedLength > 0 {
		// Do() will close the body ReadCloser when it is done
		// with it.
		req.Body = body
	} else {
		// "For client requests, a value of 0 means unknown if Body is
		// not nil."  In this case we do want the body to be empty, so
		// don't set req.Body.  However, we still need to close the
		// body ReadCloser.
		body.Close()
	}

	req.Header.Add("Authorization", fmt.Sprintf("OAuth2 %s", this.Arvados.ApiToken))
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add(X_Keep_Desired_Replicas, fmt.Sprint(this.Want_replicas))

	var resp *http.Response
	if resp, err = this.httpClient().Do(req); err != nil {
		DebugPrintf("DEBUG: [%08x] Upload failed %v error: %v", requestID, url, err.Error())
		upload_status <- uploadStatus{err, url, 0, 0, ""}
		return
	}

	rep := 1
	if xr := resp.Header.Get(X_Keep_Replicas_Stored); xr != "" {
		fmt.Sscanf(xr, "%d", &rep)
	}

	defer resp.Body.Close()
	defer io.Copy(ioutil.Discard, resp.Body)

	respbody, err2 := ioutil.ReadAll(&io.LimitedReader{R: resp.Body, N: 4096})
	response := strings.TrimSpace(string(respbody))
	if err2 != nil && err2 != io.EOF {
		DebugPrintf("DEBUG: [%08x] Upload %v error: %v response: %v", requestID, url, err2.Error(), response)
		upload_status <- uploadStatus{err2, url, resp.StatusCode, rep, response}
	} else if resp.StatusCode == http.StatusOK {
		DebugPrintf("DEBUG: [%08x] Upload %v success", requestID, url)
		upload_status <- uploadStatus{nil, url, resp.StatusCode, rep, response}
	} else {
		if resp.StatusCode >= 300 && response == "" {
			response = resp.Status
		}
		DebugPrintf("DEBUG: [%08x] Upload %v error: %v response: %v", requestID, url, resp.StatusCode, response)
		upload_status <- uploadStatus{errors.New(resp.Status), url, resp.StatusCode, rep, response}
	}
}

func (this *KeepClient) putReplicas(
	hash string,
	tr *streamer.AsyncStream,
	expectedLength int64) (locator string, replicas int, err error) {

	// Generate an arbitrary ID to identify this specific
	// transaction in debug logs.
	requestID := rand.Int31()

	// Calculate the ordering for uploading to servers
	sv := NewRootSorter(this.WritableLocalRoots(), hash).GetSortedRoots()

	// The next server to try contacting
	next_server := 0

	// The number of active writers
	active := 0

	// Used to communicate status from the upload goroutines
	upload_status := make(chan uploadStatus)
	defer func() {
		// Wait for any abandoned uploads (e.g., we started
		// two uploads and the first replied with replicas=2)
		// to finish before closing the status channel.
		go func() {
			for active > 0 {
				<-upload_status
			}
			close(upload_status)
		}()
	}()

	replicasDone := 0
	replicasTodo := this.Want_replicas

	replicasPerThread := this.replicasPerService
	if replicasPerThread < 1 {
		// unlimited or unknown
		replicasPerThread = replicasTodo
	}

	retriesRemaining := 1 + this.Retries
	var retryServers []string

	lastError := make(map[string]string)

	for retriesRemaining > 0 {
		retriesRemaining -= 1
		next_server = 0
		retryServers = []string{}
		for replicasTodo > 0 {
			for active*replicasPerThread < replicasTodo {
				// Start some upload requests
				if next_server < len(sv) {
					DebugPrintf("DEBUG: [%08x] Begin upload %s to %s", requestID, hash, sv[next_server])
					go this.uploadToKeepServer(sv[next_server], hash, tr.MakeStreamReader(), upload_status, expectedLength, requestID)
					next_server += 1
					active += 1
				} else {
					if active == 0 && retriesRemaining == 0 {
						msg := "Could not write sufficient replicas: "
						for _, resp := range lastError {
							msg += resp + "; "
						}
						msg = msg[:len(msg)-2]
						return locator, replicasDone, InsufficientReplicasError(errors.New(msg))
					} else {
						break
					}
				}
			}
			DebugPrintf("DEBUG: [%08x] Replicas remaining to write: %v active uploads: %v",
				requestID, replicasTodo, active)

			// Now wait for something to happen.
			if active > 0 {
				status := <-upload_status
				active -= 1

				if status.statusCode == 200 {
					// good news!
					replicasDone += status.replicas_stored
					replicasTodo -= status.replicas_stored
					locator = status.response
					delete(lastError, status.url)
				} else {
					msg := fmt.Sprintf("[%d] %s", status.statusCode, status.response)
					if len(msg) > 100 {
						msg = msg[:100]
					}
					lastError[status.url] = msg
				}

				if status.statusCode == 0 || status.statusCode == 408 || status.statusCode == 429 ||
					(status.statusCode >= 500 && status.statusCode != 503) {
					// Timeout, too many requests, or other server side failure
					// Do not retry when status code is 503, which means the keep server is full
					retryServers = append(retryServers, status.url[0:strings.LastIndex(status.url, "/")])
				}
			} else {
				break
			}
		}

		sv = retryServers
	}

	return locator, replicasDone, nil
}
