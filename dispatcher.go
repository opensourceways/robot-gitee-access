package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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

func (d *dispatcher) wait() {
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

	if err := d.dispatch(eventType, payload, r.Header, l); err != nil {
		l.WithError(err).Error()
	}
}

func (d *dispatcher) dispatch(eventType string, payload []byte, h http.Header, l *logrus.Entry) error {
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

	l = l.WithFields(logrus.Fields{
		"org":        org,
		"repo":       repo,
		"event type": eventType,
	})

	endpoints := d.agent.getEndpoints(org, repo, eventType)
	l.WithField("endpoints", strings.Join(endpoints, ", ")).Info("start dispatching event.")

	d.doDispatch(endpoints, payload, h, l)
	return nil
}

func (d *dispatcher) doDispatch(endpoints []string, payload []byte, h http.Header, l *logrus.Entry) {
	h.Set("User-Agent", "Robot-Gitee-Access")

	newReq := func(endpoint string) (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
		if err != nil {
			return nil, err
		}

		req.Header = h

		return req, nil
	}

	reqs := make([]*http.Request, 0, len(endpoints))

	for _, endpoint := range endpoints {
		if req, err := newReq(endpoint); err == nil {
			reqs = append(reqs, req)
		} else {
			l.WithError(err).WithField("endpoint", endpoint).Error("Error generating http request.")
		}
	}

	for _, req := range reqs {
		d.wg.Add(1)

		// concurrent action is sending request not generating it.
		// so, generates requests first.
		go func(req *http.Request) {
			defer d.wg.Done()

			if err := d.forwardTo(req); err != nil {
				l.WithError(err).WithField("endpoint", req.URL.String()).Error("Error forwarding event.")
			}
		}(req)
	}
}

func (d *dispatcher) forwardTo(req *http.Request) error {
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
	if resp, err = d.ec.Do(req); err == nil {
		return
	}

	maxRetries := 4
	backoff := 100 * time.Millisecond

	for retries := 0; retries < maxRetries; retries++ {
		time.Sleep(backoff)
		backoff *= 2

		if resp, err = d.ec.Do(req); err == nil {
			break
		}
	}
	return
}
