package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IceWhaleTech/CasaOS-Common/model"
	conf "github.com/LinkLeong/zima_cron/internal/config"
	svc "github.com/LinkLeong/zima_cron/internal/service"
)

type Task struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Command    string        `json:"command"`
	Type       string        `json:"type"`
	Interval   time.Duration `json:"interval_ms"`
	CronExpr   string        `json:"cron_expr"`
	Status     string        `json:"status"`
	NextRunAt  int64         `json:"next_run_at"`
	LastRunAt  int64         `json:"last_run_at"`
	LastResult *Result       `json:"last_result"`
	logs       []LogEntry
	timer      *time.Timer
	ticker     *time.Ticker
}

type Result struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
type LogEntry struct {
	Time       int64  `json:"time"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
}

var (
	tasks = map[string]*Task{}
	mu    sync.Mutex
)

func main() {
	svc.Initialize(conf.CommonInfo.RuntimePath)
	mux := http.NewServeMux()
	mux.HandleFunc("/zima_cron/tasks", withCORS(tasksHandler))
	mux.HandleFunc("/zima_cron/tasks/", withCORS(taskActionHandler))
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
	if err != nil {
		log.Fatal(err)
	}
	if svc.Gateway != nil {
		if err := svc.Gateway.CreateRoute(&model.Route{Path: "/zima_cron", Target: "http://" + listener.Addr().String()}); err != nil {
			log.Fatal(err)
		}
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("zima-cron backend listening on http://%s", listener.Addr().String())
	if err := srv.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

type createReq struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Type        string `json:"type"`
	IntervalMin int    `json:"interval_min"`
	CronExpr    string `json:"cron_expr"`
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mu.Lock()
		defer mu.Unlock()
		out := make([]*Task, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, sanitizeTask(t))
		}
		json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Command) == "" {
			http.Error(w, "name/command required", 400)
			return
		}
		if req.Type != "interval" && req.Type != "cron" {
			http.Error(w, "invalid type", 400)
			return
		}
		id := strconv.FormatInt(time.Now().UnixNano(), 10)
		t := &Task{ID: id, Name: req.Name, Command: req.Command, Type: req.Type, Status: "running", CronExpr: ""}
		if req.Type == "interval" {
			if req.IntervalMin < 1 {
				http.Error(w, "interval_min >=1", 400)
				return
			}
			t.Interval = time.Duration(req.IntervalMin) * time.Minute
		} else {
			if !isValidCron(req.CronExpr) {
				http.Error(w, "invalid cron", 400)
				return
			}
			t.CronExpr = req.CronExpr
		}
		mu.Lock()
		tasks[id] = t
		mu.Unlock()
		startSchedule(t)
		json.NewEncoder(w).Encode(sanitizeTask(t))
	default:
		w.WriteHeader(405)
	}
}

func taskActionHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/zima_cron/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		w.WriteHeader(404)
		return
	}
	id := parts[0]
	mu.Lock()
	t := tasks[id]
	mu.Unlock()
	if t == nil {
		w.WriteHeader(404)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(sanitizeTask(t))
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		mu.Lock()
		clearSchedule(t)
		delete(tasks, id)
		mu.Unlock()
		w.WriteHeader(204)
		return
	}
	if len(parts) < 2 {
		w.WriteHeader(404)
		return
	}
	action := parts[1]
	switch action {
	case "run":
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		runTaskOnce(t)
		json.NewEncoder(w).Encode(sanitizeTask(t))
	case "toggle":
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		toggleTask(t)
		json.NewEncoder(w).Encode(sanitizeTask(t))
	case "logs":
		if r.Method == http.MethodGet {
			mu.Lock()
			logs := append([]LogEntry(nil), t.logs...)
			mu.Unlock()
			if logs == nil {
				logs = []LogEntry{}
			}
			json.NewEncoder(w).Encode(logs)
		} else if r.Method == http.MethodPost && len(parts) >= 3 && parts[2] == "clear" {
			mu.Lock()
			t.logs = nil
			mu.Unlock()
			w.WriteHeader(204)
		} else {
			w.WriteHeader(405)
		}
	default:
		w.WriteHeader(404)
	}
}

func sanitizeTask(t *Task) *Task {
	cp := *t
	cp.logs = nil
	cp.ticker = nil
	cp.timer = nil
	return &cp
}

func startSchedule(t *Task) {
	clearSchedule(t)
	if t.Type == "interval" {
		t.ticker = time.NewTicker(t.Interval)
		t.NextRunAt = time.Now().Add(t.Interval).UnixMilli()
		go func(id string) {
			for range t.ticker.C {
				mu.Lock()
				tt := tasks[id]
				mu.Unlock()
				if tt == nil || tt.Status != "running" {
					continue
				}
				runTaskOnce(tt)
				tt.NextRunAt = time.Now().Add(tt.Interval).UnixMilli()
			}
		}(t.ID)
	} else {
		scheduleCronNext(t)
	}
}

func clearSchedule(t *Task) {
	if t.ticker != nil {
		t.ticker.Stop()
		t.ticker = nil
	}
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}

func toggleTask(t *Task) {
	if t.Status == "running" {
		t.Status = "paused"
		clearSchedule(t)
		t.NextRunAt = 0
	} else {
		t.Status = "running"
		startSchedule(t)
	}
}

func runTaskOnce(t *Task) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", t.Command)
	out, err := cmd.CombinedOutput()
	finished := time.Now()
	success := err == nil && ctx.Err() == nil
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		if err != nil {
			msg = err.Error()
		} else {
			msg = "执行完成"
		}
	}
	if len(msg) > 4000 {
		msg = msg[:4000] + "..."
	}
	t.LastRunAt = finished.UnixMilli()
	t.LastResult = &Result{Success: success, Message: msg}
	t.logs = append([]LogEntry{{Time: t.LastRunAt, DurationMs: finished.Sub(start).Milliseconds(), Success: success, Message: msg}}, t.logs...)
}

func isValidCron(expr string) bool { return len(strings.Fields(expr)) == 5 }

func scheduleCronNext(t *Task) {
	next := cronNext(t.CronExpr, time.Now())
	if next.IsZero() {
		return
	}
	delay := time.Until(next)
	if delay < 0 {
		delay = 0
	}
	t.NextRunAt = next.UnixMilli()
	t.timer = time.AfterFunc(delay, func() {
		if t.Status == "running" {
			runTaskOnce(t)
			scheduleCronNext(t)
		}
	})
}

func cronNext(expr string, from time.Time) time.Time {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return time.Time{}
	}
	minSet := parseCronField(f[0], 0, 59, false)
	hourSet := parseCronField(f[1], 0, 23, false)
	domSet := parseCronField(f[2], 1, 31, false)
	monSet := parseCronField(f[3], 1, 12, false)
	dowSet := parseCronField(f[4], 0, 6, true)
	d := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 100000; i++ {
		m := d.Minute()
		h := d.Hour()
		dom := d.Day()
		mon := int(d.Month())
		dow := int(d.Weekday())
		if dow == 0 && dowSet.has7 {
			dow = 7
		}
		minuteOk := minSet.set[m]
		hourOk := hourSet.set[h]
		monthOk := monSet.set[mon]
		domOk := domSet.set[dom]
		dowOk := dowSet.set[dow]
		dayOk := (domSet.isAll && dowSet.isAll) || (domSet.isAll && dowOk) || (dowSet.isAll && domOk) || (domOk || dowOk)
		if minuteOk && hourOk && monthOk && dayOk {
			return d
		}
		d = d.Add(time.Minute)
	}
	return time.Time{}
}

type cronField struct {
	set   map[int]bool
	isAll bool
	has7  bool
}

func parseCronField(expr string, min, max int, isDow bool) cronField {
	cf := cronField{set: map[int]bool{}}
	tokens := strings.Split(strings.ToLower(strings.TrimSpace(expr)), ",")
	addRange := func(a, b, step int) {
		if step <= 0 {
			step = 1
		}
		for v := a; v <= b; v += step {
			cf.set[v] = true
		}
	}
	aliases := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "*" {
			cf.isAll = true
			addRange(min, max, 1)
			continue
		}
		if strings.HasPrefix(tok, "*/") {
			step, _ := strconv.Atoi(strings.TrimPrefix(tok, "*/"))
			cf.isAll = true
			addRange(min, max, step)
			continue
		}
		if isDow {
			if v, ok := aliases[tok]; ok {
				cf.set[v] = true
				continue
			}
		}
		if strings.Contains(tok, "-") {
			parts := strings.Split(tok, "-")
			a, _ := strconv.Atoi(parts[0])
			bPart := parts[1]
			step := 1
			if strings.Contains(bPart, "/") {
				sub := strings.Split(bPart, "/")
				bPart = sub[0]
				step, _ = strconv.Atoi(sub[1])
			}
			b, _ := strconv.Atoi(bPart)
			if isDow && b == 7 {
				cf.has7 = true
			}
			addRange(int(math.Max(float64(min), float64(a))), int(math.Min(float64(max), float64(b))), step)
			continue
		}
		if strings.Contains(tok, "/") {
			parts := strings.Split(tok, "/")
			if parts[0] == "*" {
				step, _ := strconv.Atoi(parts[1])
				addRange(min, max, step)
				continue
			}
		}
		v, err := strconv.Atoi(tok)
		if err == nil {
			if isDow && v == 7 {
				cf.has7 = true
			}
			if v >= min && v <= max {
				cf.set[v] = true
			}
		}
	}
	return cf
}
