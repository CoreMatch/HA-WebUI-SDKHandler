// HA-WebUI-SDKHandler 是前端编译期 SDK 聚合器。
//
// 职责：只从 HRPAuth 主服务拉取 SDK 源码头包，注入前端模板，
// 聚合生成静态前端并编译，成功后原子发布。
//
// 启动后进入轮询协程（hrpauth.poll_interval_seconds），发现 SDK 列表变化
// 即触发一次构建；主动构建通过 POST /api/build/trigger。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ha-webui-sdkhandler/internal/broker"
	"ha-webui-sdkhandler/internal/build"
	"ha-webui-sdkhandler/internal/config"
	"ha-webui-sdkhandler/internal/inject"
	"ha-webui-sdkhandler/internal/publish"
	"ha-webui-sdkhandler/internal/state"
)

// errNoChange 表示列表无变化、无需构建（非错误，用于跳过）。
var errNoChange = errors.New("SDK 列表无变化，跳过构建")

// app 聚合服务运行时依赖。
type app struct {
	cfg          *config.Config
	store        *state.Store
	broker       *broker.Client
	orchestrator *orchestrator

	// 构建互斥：防止轮询与 trigger 并发重复构建。
	buildMu sync.Mutex
	// status 保存最近一次构建状态（供 API 读取）。
	status statusHolder
}

// statusHolder 线程安全保存最近一次构建状态（供 API 读取）。
type statusHolder struct {
	mu     sync.RWMutex
	latest *state.BuildRecord
}

func (s *statusHolder) set(rec *state.BuildRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = rec
}

func (s *statusHolder) get() *state.BuildRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return nil
	}
	cp := *s.latest
	return &cp
}

// orchestrator 执行一次完整构建管线：拉包 -> 复制模板 -> 注入 -> 编译 -> 发布。
type orchestrator struct {
	cfg    *config.Config
	store  *state.Store
	broker *broker.Client
	status *statusHolder
}

// runOnce 拉取最新 SDK 列表并构建。changed=false 表示列表无变化（无需构建）。
func (o *orchestrator) runOnce(force bool) (bool, error) {
	list, err := o.broker.List()
	if err != nil {
		return false, fmt.Errorf("轮询 SDK 包列表失败: %w", err)
	}
	if !force && !list.Changed {
		return false, errNoChange
	}

	// 下载所有包（下载失败视为本次轮询失败，但保留上一版 current）。
	archives := map[string]string{}
	hashes := map[string]string{}
	runScripts := map[string]bool{}
	for _, pkg := range list.Packages {
		path, sum, err := o.broker.Download(pkg.Name)
		if err != nil {
			return true, fmt.Errorf("下载 SDK 包 %s 失败: %w", pkg.Name, err)
		}
		archives[pkg.Name] = path
		hashes[pkg.Name] = sum
		runScripts[pkg.Name] = pkg.RunScripts
	}
	if len(archives) == 0 {
		// 列表为空：仍构建一次空前端（全量模板 + 无 SDK）。
		runScripts = map[string]bool{}
	}

	buildID := build.NewBuildID(time.Now())
	workDir := filepath.Join(o.cfg.Build.WorkDir, "run-"+buildID)

	rec := state.BuildRecord{
		BuildID:      buildID,
		Status:       state.StatusBuilding,
		StartedAt:    time.Now(),
		SDKVersion:   list.ETag,
		ManifestSums: hashes,
	}
	for _, p := range list.Packages {
		rec.SDKList = append(rec.SDKList, p.Name)
	}
	if err := o.store.Record(rec); err != nil {
		return true, fmt.Errorf("记录构建状态失败: %w", err)
	}
	o.status.set(&rec)

	err = o.execute(buildID, workDir, archives, hashes, runScripts, &rec)
	return true, err
}

// execute 执行注入 + 编译 + 发布；任一步失败回滚（删除 work 目录）。
func (o *orchestrator) execute(buildID, workDir string, archives, hashes map[string]string, runScripts map[string]bool, rec *state.BuildRecord) error {
	cleanup := func() {
		// 失败路径：删除本次 work 目录，保留上一版 current。
		_ = os.RemoveAll(workDir)
		_ = os.RemoveAll(filepath.Join(o.cfg.Build.WorkDir, "run-"+buildID))
	}

	// 1. 复制模板（只读模板 → 全新 work 目录，全量复制）。
	if err := inject.CopyTemplate(o.cfg.Build.TemplateDir, workDir); err != nil {
		rec.Status = state.StatusFailed
		rec.Error = "模板复制失败: " + err.Error()
		now := time.Now()
		rec.FinishedAt = &now
		_ = o.store.Record(*rec)
		o.status.set(rec)
		cleanup()
		return fmt.Errorf("复制模板失败: %w", err)
	}

	// 2. 注入 SDK。
	injRes, err := inject.Inject(inject.Options{
		WorkDir:              workDir,
		PackageArchives:      archives,
		PackageHashes:        hashes,
		IgnoreScriptsDefault: o.cfg.Build.IgnoreScriptsDefault,
		RunScripts:           runScripts,
	})
	if err != nil {
		rec.Status = state.StatusFailed
		rec.Error = "注入失败: " + err.Error()
		now := time.Now()
		rec.FinishedAt = &now
		_ = o.store.Record(*rec)
		o.status.set(rec)
		cleanup()
		return fmt.Errorf("注入失败: %w", err)
	}
	rec.SDKVersion = injRes.SDKVersion
	rec.SDKList = injRes.SDKList
	_ = o.store.Record(*rec)

	// 3. 编译。
	buildRes := build.Run(build.Options{
		WorkDir:              workDir,
		NPMCommand:           o.cfg.Build.NPMCommand,
		IgnoreScriptsDefault: o.cfg.Build.IgnoreScriptsDefault,
		RunScripts:           injRes.RunScripts,
		NPMBlacklist:         o.cfg.Build.NPMBlacklist,
		Timeout:              time.Duration(o.cfg.Build.BuildTimeout) * time.Second,
		Logf: func(format string, args ...any) {
			log.Printf("[build:"+buildID+"] "+format, args...)
		},
	})
	if !buildRes.OK {
		rec.Status = state.StatusFailed
		rec.Error = buildRes.Err
		now := time.Now()
		rec.FinishedAt = &now
		_ = o.store.Record(*rec)
		o.status.set(rec)
		cleanup()
		return fmt.Errorf("构建失败: %s", buildRes.Err)
	}

	// 4. 发布（原子切换 current）。
	now := time.Now()
	pubRec := *rec
	pubRec.Status = state.StatusSuccess
	pubRec.FinishedAt = &now
	if _, err := publish.Publish(publish.Options{
		ArtifactsDir: o.cfg.Build.ArtifactsDir,
		CurrentLink:  o.cfg.Build.CurrentLink,
		WorkDir:      workDir,
		BuildID:      buildID,
		Record:       pubRec,
		Store:        o.store,
	}); err != nil {
		rec.Status = state.StatusFailed
		rec.Error = "发布失败: " + err.Error()
		now2 := time.Now()
		rec.FinishedAt = &now2
		_ = o.store.Record(*rec)
		o.status.set(rec)
		cleanup()
		return fmt.Errorf("发布失败: %w", err)
	}
	// 发布成功：更新内存状态中的成功记录。
	successRec := *rec
	successRec.Status = state.StatusSuccess
	successRec.FinishedAt = &now
	o.status.set(&successRec)
	// 清理本次 work 目录（发布后不再需要）。
	_ = os.RemoveAll(workDir)
	log.Printf("[build:%s] 构建并发布成功", buildID)
	return nil
}

func (o *orchestrator) preRunCheck() error {
	if err := o.ensureTemplateRepo(); err != nil {
		return err
	}
	if _, err := os.Stat(o.cfg.Build.TemplateDir); err != nil {
		return fmt.Errorf("模板目录不可用: %w（请配置 build.template_dir 指向 HRPAuth-Web 仓库）", err)
	}
	return nil
}

// ensureTemplateRepo 检查模板目录是否为空，如果为空则拉取 git 仓库。
func (o *orchestrator) ensureTemplateRepo() error {
	dir := o.cfg.Build.TemplateDir
	url := o.cfg.Build.TemplateGitURL

	if url == "" {
		return nil
	}

	empty, err := isEmptyDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查模板目录失败: %w", err)
	}

	if os.IsNotExist(err) || empty {
		log.Printf("检测到模板目录 %s 为空或不存在，开始从 %s 拉取最新仓库...", dir, url)
		// 如果父目录不存在，先创建父目录
		parent := filepath.Dir(dir)
		if _, err := os.Stat(parent); os.IsNotExist(err) {
			if err := os.MkdirAll(parent, 0755); err != nil {
				return fmt.Errorf("创建模板父目录失败: %w", err)
			}
		}

		// 执行 git clone
		cmd := exec.Command("git", "clone", url, dir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone 失败: %w, 输出: %s", err, string(output))
		}
		log.Printf("模板仓库拉取成功")
	}

	return nil
}

func isEmptyDir(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // 尝试读取一个文件名
	if err == io.EOF {
		return true, nil // 读不到说明为空
	}
	return false, err // 读到了或者出错
}

// pollLoop 按 poll_interval_seconds 轮询 SDK 列表，变化即构建。
func (a *app) pollLoop(ctx context.Context) {
	interval := time.Duration(a.cfg.HRPAuth.PollIntervalSeconds) * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
		if err := a.triggerBuild(ctx, false); err != nil {
			// errNoChange 是正常跳过，不记日志；其余视为轮询失败。
			if !errors.Is(err, errNoChange) {
				log.Printf("轮询触发构建失败: %v", err)
			}
		}
	}
}

// triggerBuild 触发一次构建；force=true 时忽略 ETag 变化直接重建。
// 用互斥锁保证并发安全：构建期间再次触发返回 busy。
func (a *app) triggerBuild(ctx context.Context, force bool) error {
	if !a.buildMu.TryLock() {
		return errors.New("构建进行中，已返回 busy（请稍后重试）")
	}
	defer a.buildMu.Unlock()

	if err := a.orchestrator.preRunCheck(); err != nil {
		return err
	}
	_, err := a.orchestrator.runOnce(force)
	if errors.Is(err, errNoChange) {
		return errNoChange
	}
	return err
}

func main() {
	// 配置路径：默认 ./config.yaml，可用环境变量 SDKHANDLER_CONFIG 覆盖。
	cfgPath := os.Getenv("SDKHANDLER_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	store, err := state.New(cfg.StateDir)
	if err != nil {
		log.Fatalf("初始化状态存储失败: %v", err)
	}
	client, err := broker.New(
		cfg.HRPAuth.BaseURL,
		cfg.HRPAuth.ClientID,
		cfg.HRPAuth.ClientSecret,
		filepath.Join(cfg.StateDir, "cache"),
		30*time.Second,
	)
	if err != nil {
		log.Fatalf("初始化 HRPAuth 客户端失败: %v", err)
	}

	a := &app{
		cfg:    cfg,
		store:  store,
		broker: client,
	}
	a.orchestrator = &orchestrator{cfg: cfg, store: store, broker: client}
	a.orchestrator.status = &a.status

	// 启动前检查并拉取模板
	if err := a.orchestrator.preRunCheck(); err != nil {
		log.Printf("启动前检查失败: %v", err)
	}

	// 启动轮询协程（后台）。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.pollLoop(ctx)

	// HTTP API。
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/build/status", a.handleBuildStatus)
	r.POST("/api/build/trigger", a.handleBuildTrigger)
	r.GET("/api/revision", a.handleRevision)

	if cfg.ServeArtifacts {
		// 静态托管 current_link 产物（默认关闭）。
		r.GET("/", a.handleStaticIndex)
		r.NoRoute(a.handleStatic)
	}

	srv := &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: r,
	}

	// 优雅退出。
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("收到退出信号，优雅关闭...")
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		_ = srv.Shutdown(ctx2)
		cancel()
	}()

	log.Printf("HA-WebUI-SDKHandler 启动，监听 %s（轮询间隔 %ds）",
		cfg.Server.ListenAddr, cfg.HRPAuth.PollIntervalSeconds)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务异常退出: %v", err)
	}
}

// handleBuildStatus 返回最近一次构建结果。
func (a *app) handleBuildStatus(c *gin.Context) {
	rec := a.store.Latest()
	if rec == nil {
		c.JSON(http.StatusOK, gin.H{"state": "empty", "build_id": "", "sdk_list": []string{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"state":    rec.Status,
		"build_id": rec.BuildID,
		"time":     rec.StartedAt,
		"sdk_list": rec.SDKList,
		"error":    rec.Error,
		"artifact": rec.ArtifactDir,
	})
}

// handleBuildTrigger 主动触发一次构建；构建中返回 409 busy。
func (a *app) handleBuildTrigger(c *gin.Context) {
	if err := a.triggerBuild(c.Request.Context(), false); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	rec := a.store.Latest()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "构建完成", "build": rec})
}

// handleRevision 返回当前生效产物版本信息。
func (a *app) handleRevision(c *gin.Context) {
	info, err := publish.LoadRevision(a.store, a.cfg.Build.CurrentLink)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (a *app) handleStaticIndex(c *gin.Context) {
	cur, err := publish.ResolveCurrent(a.cfg.Build.CurrentLink)
	if err != nil || cur == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "无已发布产物"})
		return
	}
	c.File(filepath.Join(cur, "index.html"))
}

func (a *app) handleStatic(c *gin.Context) {
	cur, err := publish.ResolveCurrent(a.cfg.Build.CurrentLink)
	if err != nil || cur == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "无已发布产物"})
		return
	}
	file := filepath.Join(cur, c.Param("filepath"))
	c.File(file)
}
