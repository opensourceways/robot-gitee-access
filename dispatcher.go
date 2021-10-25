package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/opensourceways/community-robot-lib/giteeclient"
	"github.com/sirupsen/logrus"
)

type dispatcher struct {
	agent *demuxConfigAgent

	hmac func() string

	// ec is an http client used for dispatching events
	// to external plugin services.
	ec http.Client
	// Tracks running handlers for graceful shutdown
	wg sync.WaitGroup
}

func (d *dispatcher) Wait() {
	d.wg.Wait() // Handle remaining requests
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (d *dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, _, ok := giteeclient.ValidateWebhook(w, r, d.hmac)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	l := logrus.WithFields(
		logrus.Fields{
			"event-type": eventType,
			"event-id":   eventGUID,
		},
	)

	if err := d.Dispatch(eventType, payload, r.Header, l); err != nil {
		l.WithError(err).Error()
	}
}

func (d *dispatcher) Dispatch(eventType string, payload []byte, h http.Header, l *logrus.Entry) error {
	org := ""
	repo := ""

	switch eventType {
	case giteeclient.EventTypeNote:
		e, err := giteeclient.ConvertToNoteEvent(payload)
		if err != nil {
			return err
		}

		org, repo = giteeclient.GetOwnerAndRepoByNoteEvent(&e)

	case giteeclient.EventTypeIssue:
		e, err := giteeclient.ConvertToIssueEvent(payload)
		if err != nil {
			return err
		}

		org, repo = giteeclient.GetOwnerAndRepoByIssueEvent(&e)

	case giteeclient.EventTypePR:
		e, err := giteeclient.ConvertToPREvent(payload)
		if err != nil {
			return err
		}

		org, repo = giteeclient.GetOwnerAndRepoByPREvent(&e)

	case giteeclient.EventTypePush:
		e, err := giteeclient.ConvertToPushEvent(payload)
		if err != nil {
			return err
		}

		org, repo = giteeclient.GetOwnerAndRepoByPushEvent(&e)

	default:
		l.Debug("Ignoring unknown event type")
		return nil
	}

	d.dispatch(d.getEndpoints(org, repo, eventType), payload, h, l)
	return nil
}

func (d *dispatcher) getEndpoints(org, repo, event string) []string {
	return d.agent.GetEndpoints(org, repo, event)
}

func (d *dispatcher) dispatch(endpoints []string, payload []byte, h http.Header, l *logrus.Entry) {
	h.Set("User-Agent", "Robot-Gitee-Access")

	for _, p := range endpoints {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			if err := d.forwardTo(p, payload, h); err != nil {
				l.WithError(err).WithField("endpoint", p).Error("Error dispatching event.")
			}
		}()
	}
}

// forwardTo creates a new request using the provided payload and headers
// and sends the request forward to the provided endpoint.
func (d *dispatcher) forwardTo(endpoint string, payload []byte, h http.Header) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header = h

	resp, err := d.do(req)
	if err != nil || resp == nil {
		return err
	}

	defer resp.Body.Close()

	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("response has status %q and body %q", resp.Status, string(rb))
	}
	return nil
}

func (d *dispatcher) do(req *http.Request) (resp *http.Response, err error) {
	maxRetries := 5
	backoff := 100 * time.Millisecond

	for retries := 0; retries < maxRetries; retries++ {
		if resp, err = d.ec.Do(req); err == nil {
			break
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return
}
