package auth

import (
	"sync"
	"time"
)

// Reserve charges the IP before dialing and keeps account attempts in flight.
func (t *Throttle) Reserve(account, ip string, max int, cooldown time.Duration) (func(bool), bool) {
	t.mu.Lock()
	now := time.Now()
	for _, key := range []string{account, ip} {
		if e := t.entries[key]; e != nil && now.After(e.expiresAt) && e.inFlight == 0 {
			delete(t.entries, key)
		}
	}
	a, i := t.entries[account], t.entries[ip]
	if a != nil && now.After(a.expiresAt) {
		a.attempts = 0
	}
	missing := 0
	if a == nil {
		missing++
	}
	if i == nil {
		missing++
	}
	if max < 1 || len(t.entries)+missing > maxThrottleEntries || (a != nil && a.attempts+a.inFlight >= max) || (i != nil && i.attempts >= max*4) {
		t.mu.Unlock()
		return nil, false
	}
	if a == nil {
		a = &throttleEntry{}
		t.entries[account] = a
	}
	if i == nil {
		i = &throttleEntry{}
		t.entries[ip] = i
	}
	a.inFlight++
	if a.expiresAt.IsZero() || now.After(a.expiresAt) {
		a.expiresAt = now.Add(cooldown)
	}
	i.attempts++
	i.expiresAt = now.Add(cooldown)
	t.mu.Unlock()
	var once sync.Once
	return func(failed bool) {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			a.inFlight--
			if failed {
				a.attempts++
				a.expiresAt = time.Now().Add(cooldown)
			}
		})
	}, true
}

func (s *Store) Reserve(ip string, maxSessions, maxPerIP, maxConnecting, maxConnectingIP int, credits ...string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connecting == nil {
		s.connecting = make(map[string]int)
	}
	count, active, total := 0, 0, len(s.sessions)
	for id, sess := range s.sessions {
		if len(credits) > 0 && id == credits[0] {
			total--
			continue
		}
		if sess.GetString("clientIP") == ip {
			count++
		}
	}
	for _, n := range s.connecting {
		active += n
	}
	if total+active >= maxSessions || count+s.connecting[ip] >= maxPerIP || active >= maxConnecting || s.connecting[ip] >= maxConnectingIP {
		return nil, false
	}
	s.connecting[ip]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.connecting[ip]--
			if s.connecting[ip] == 0 {
				delete(s.connecting, ip)
			}
		})
	}, true
}

func (s *Store) SetExpiry(id string, expiry time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[id]; sess != nil {
		sess.ExpiresAt = expiry
		sess.hardExpiry = expiry
	}
}
