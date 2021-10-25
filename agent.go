package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/opensourceways/community-robot-lib/config"
	"github.com/opensourceways/community-robot-lib/utils"
	"github.com/sirupsen/logrus"
)

type demuxConfigAgent struct {
	agent *config.ConfigAgent

	mut     sync.RWMutex
	demux   map[string]eventsDemux
	version string
	t       utils.Timer
}

func (ca *demuxConfigAgent) load() {
	v, c := ca.agent.GetConfig()
	if ca.version == v {
		return
	}

	nc, ok := c.(*configuration)
	if !ok {
		logrus.Errorf("can't convert to configuration")
		return
	}

	if nc == nil {
		logrus.Error("empty pointer of configuration")
		return
	}

	m := nc.Config.getDemux()

	ca.version = v

	ca.mut.Lock()
	ca.demux = m
	ca.mut.Unlock()
}

func (ca *demuxConfigAgent) GetEndpoints(org, repo, event string) []string {
	ca.mut.RLock()
	v := getEndpoints(org, repo, event, ca.demux)
	ca.mut.RUnlock()

	return v
}

func getEndpoints(org, repo, event string, demux map[string]eventsDemux) []string {
	if demux == nil {
		return nil
	}

	items, ok := demux[org]
	if !ok {
		fullname := fmt.Sprintf("%s/%s", org, repo)
		if items, ok = demux[fullname]; !ok {
			return nil
		}
	}

	if items != nil {
		return items[event]
	}
	return nil
}

func (ca *demuxConfigAgent) Start() {
	ca.load()

	ca.t.Start(
		func() {
			ca.load()
		},
		1*time.Minute,
	)
}

func (ca *demuxConfigAgent) Stop() {
	ca.t.Stop()
}
