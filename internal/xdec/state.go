package xdec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ctxpkg "req/internal/ctx"
)

const stateVersion = 1

type stateStore struct {
	Path string
	Data stateFile
}

type stateFile struct {
	Version int                  `json:"version"`
	Runs    map[string]*runState `json:"runs"`
}

type runState struct {
	SourceHash string                     `json:"source_hash"`
	SourceName string                     `json:"source_name"`
	Status     string                     `json:"status"`
	ParentLog  int64                      `json:"parent_log_id,omitempty"`
	UpdatedAt  string                     `json:"updated_at"`
	Candidates map[string]*candidateState `json:"candidates"`
}

type candidateState struct {
	Username  string                   `json:"username,omitempty"`
	Scope     string                   `json:"scope,omitempty"`
	Status    string                   `json:"status"`
	Recovered string                   `json:"recovered,omitempty"`
	Attempts  map[string]*attemptState `json:"attempts"`
}

type attemptState struct {
	Status       string `json:"status"`
	Backend      string `json:"backend"`
	Format       string `json:"format"`
	WordlistID   string `json:"wordlist_id"`
	WordlistHash string `json:"wordlist_hash"`
	UpdatedAt    string `json:"updated_at"`
}

func openState(workspace *ctxpkg.Workspace, source document, parent int64) (*stateStore, error) {
	path := ""
	if workspace != nil {
		path = filepath.Join(workspace.DataPath, "xdec", "state.json")
	} else {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(cache, "xdec", "state.json")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("xdec: create state directory: %w", err)
	}
	s := &stateStore{Path: path, Data: stateFile{Version: stateVersion, Runs: map[string]*runState{}}}
	if b, err := os.ReadFile(path); err == nil {
		_ = os.Chmod(path, 0600)
		if err := json.Unmarshal(b, &s.Data); err != nil {
			return nil, fmt.Errorf("xdec: read state: %w", err)
		}
		if s.Data.Runs == nil {
			s.Data.Runs = map[string]*runState{}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("xdec: read state: %w", err)
	}
	key := digest(source.Raw)
	run := s.Data.Runs[key]
	if run == nil {
		run = &runState{SourceHash: key, SourceName: source.Name, Status: "running", Candidates: map[string]*candidateState{}}
		s.Data.Runs[key] = run
	} else {
		for _, c := range run.Candidates {
			for _, a := range c.Attempts {
				if a.Status == "running" {
					a.Status = "interrupted"
				}
			}
		}
		run.Status = "running"
	}
	if parent != 0 {
		run.ParentLog = parent
	}
	run.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	return s, s.save()
}

func (s *stateStore) save() error {
	s.Data.Version = stateVersion
	b, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *stateStore) resetRun(key string, source document, parent int64) {
	s.Data.Runs[key] = &runState{
		SourceHash: key,
		SourceName: source.Name,
		Status:     "running",
		ParentLog:  parent,
		UpdatedAt:  time.Now().Format(time.RFC3339Nano),
		Candidates: map[string]*candidateState{},
	}
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func candidateKey(c candidate) string {
	return digest([]byte(c.Value + "\x00" + c.Username + "\x00" + c.Scope))
}

func attemptKey(c candidate, wl wordlist, be backend) string {
	return digest([]byte(candidateKey(c) + "\x00" + firstKind(c.Value) + "\x00" + be.Name + "\x00" + wl.ID + "\x00" + wl.Hash))
}

func (s *stateStore) candidate(runKey string, c candidate) *candidateState {
	run := s.Data.Runs[runKey]
	key := candidateKey(c)
	cs := run.Candidates[key]
	if cs == nil {
		cs = &candidateState{Username: c.Username, Scope: c.Scope, Status: "pending", Attempts: map[string]*attemptState{}}
		run.Candidates[key] = cs
	}
	if cs.Attempts == nil {
		cs.Attempts = map[string]*attemptState{}
	}
	return cs
}

func (s *stateStore) markAttempt(runKey string, c candidate, wl wordlist, be backend, status string) {
	cs := s.candidate(runKey, c)
	key := attemptKey(c, wl, be)
	cs.Attempts[key] = &attemptState{Status: status, Backend: be.Name, Format: firstKind(c.Value), WordlistID: wl.ID, WordlistHash: wl.Hash, UpdatedAt: time.Now().Format(time.RFC3339Nano)}
	cs.Status = status
	s.Data.Runs[runKey].UpdatedAt = time.Now().Format(time.RFC3339Nano)
}
