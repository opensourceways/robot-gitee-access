package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/opensourceways/community-robot-lib/config"
	"github.com/opensourceways/community-robot-lib/utils"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

type demuxConfigAgent struct {
	agent *config.ConfigAgent

	mut     sync.RWMutex
	demux   demux
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

	// this function runs in serially, and ca.version is accessed in it,
	// so, there is no need to update it under the lock.
	ca.version = v

	ca.mut.Lock()
	ca.demux = m
	ca.mut.Unlock()
}

func (ca *demuxConfigAgent) getEndpoints(org, repo, event string) []string {
	ca.mut.RLock()
	v := ca.demux.getEventsDemux(org, repo)[event]
	ca.mut.RUnlock()

	return v.UnsortedList()
}

func (ca *demuxConfigAgent) start() {
	ca.load()

	ca.t.Start(
		func() {
			ca.load()
		},
		1*time.Minute,
		0,
	)
}

func (ca *demuxConfigAgent) stop() {
	ca.t.Stop()
}

type demux struct {
	reposEventDemux         map[string]eventsDemux
	excludeReposEventsDemux map[string]eventsDemux
}

func (d demux) getEventsDemux(org, repo string) eventsDemux {
	res := eventsDemux{}
	if d.reposEventDemux == nil {
		return res
	}

	cp := func(origin eventsDemux) eventsDemux {
		r := make(eventsDemux)
		if origin == nil {
			return r
		}

		for k, v := range origin {
			r[k] = sets.NewString().Union(v)
		}

		return r
	}

	fn := fmt.Sprintf("%s/%s", org, repo)
	if v, ok := d.reposEventDemux[fn]; ok {
		res = cp(v)
	} else if v, ok := d.reposEventDemux[org]; ok {
		res = cp(v)
	}

	if d.excludeReposEventsDemux == nil {
		return res
	}

	if v, ok := d.excludeReposEventsDemux[fn]; ok {
		for k, es := range res {
			if s, ok := v[k]; ok {
				res[k] = es.Difference(s)
			}
		}
	}

	return res
}
