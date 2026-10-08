package http

import (
	stdhttp "net/http"
	"strings"
)

type Router = stdhttp.Handler

type Handlers interface {
	CreatePayment(stdhttp.ResponseWriter, *stdhttp.Request)
	GetPayment(stdhttp.ResponseWriter, *stdhttp.Request, string)
	Authorize(stdhttp.ResponseWriter, *stdhttp.Request, string)
	Capture(stdhttp.ResponseWriter, *stdhttp.Request, string)
	Fail(stdhttp.ResponseWriter, *stdhttp.Request, string)
	GetBalance(stdhttp.ResponseWriter, *stdhttp.Request, string)
	Health(stdhttp.ResponseWriter, *stdhttp.Request)
	Replay(stdhttp.ResponseWriter, *stdhttp.Request)
}

func NewRouter(h Handlers) Router {
	mux := stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		p := r.URL.Path
		m := r.Method

		if m == "POST" && p == "/payments" {
			h.CreatePayment(w, r)
			return
		}

		if m == "GET" && strings.HasPrefix(p, "/payments/") {
			h.GetPayment(w, r, strings.TrimPrefix(p, "/payments/"))
			return
		}

		if m == "POST" && strings.HasPrefix(p, "/payments/") {
			parts := strings.Split(strings.TrimPrefix(p, "/payments/"), "/")
			if len(parts) == 2 {
				pid, action := parts[0], parts[1]
				switch action {
				case "authorize":
					h.Authorize(w, r, pid)
					return
				case "capture":
					h.Capture(w, r, pid)
					return
				case "fail":
					h.Fail(w, r, pid)
					return
				}
			}
		}

		if m == "GET" && strings.HasPrefix(p, "/accounts/") && strings.HasSuffix(p, "/balance") {
			acct := strings.TrimSuffix(strings.TrimPrefix(p, "/accounts/"), "/balance")
			h.GetBalance(w, r, acct)
			return
		}

		if m == "GET" && p == "/admin/health" {
			h.Health(w, r)
			return
		}
		if m == "POST" && p == "/admin/replay" {
			h.Replay(w, r)
			return
		}

		stdhttp.NotFound(w, r)
	})

	return withRequestIDAndLogging(mux)
}
