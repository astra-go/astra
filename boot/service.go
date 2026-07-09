// Package boot 提供 Astra 服务启动脚手架 —— 配置加载、日志初始化、Health 端点、优雅关闭一站集成。
//
// 用法：
//
//	func main() {
//	    svc := boot.New("usercenter-svc",
//	        boot.WithConfigPath("config/service.yaml"),
//	        boot.WithEnvPrefix("USC"),
//	    )
//	    svc.Use(middleware.RequestID())
//	    svc.UseLogger()
//
//	    // 注册健康检查依赖
//	    svc.RegisterHealthChecker(&boot.DBChecker{DB: db})
//	    svc.RegisterHealthChecker(&boot.RedisChecker{Redis: redis})
//
//	    svc.Router(func(app *astra.App) {
//	        // 注册业务路由
//	    })
//	    svc.Run()
//	}
package boot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/backend"
	"github.com/astra-go/astra/config"
	"github.com/astra-go/astra/middleware"
)

// ============================================================================
// Boot — Service 配置结构体
// ============================================================================

// Config 是 boot 层通用的服务配置。
// 通过 YAML 文件 + 环境变量 + 编程注入三源合并，Scan 到该结构体。
// 各字段支持 default tag，缺省键自动获得默认值。
type Config struct {
	// Name 服务名，用于日志、跟踪和错误报告。
	Name string `yaml:"name" json:"name" default:"app"`
	// Port 监听端口（不含冒号），如 "8080"。
	Port string `yaml:"port" json:"port" default:"8080"`
	// Mode 运行模式：dev / prod / staging / test。
	Mode string `yaml:"mode" json:"mode" default:"dev"`
	// LogLevel 日志级别：debug / info / warn / error。
	LogLevel string `yaml:"log_level" json:"log_level" default:"info"`
	// LogFormat 日志格式：auto / json / text。
	// auto 模式下 dev → text，prod → JSON。
	LogFormat string `yaml:"log_format" json:"log_format" default:"auto"`
	// ShutdownTimeout 优雅关闭超时（秒），超时后强制退出。
	ShutdownTimeout int `yaml:"shutdown_timeout" json:"shutdown_timeout" default:"30"`
	// CORSOrigins 跨域允许来源列表，为空则不设置 CORS。
	CORSOrigins  []string     `yaml:"cors_origins" json:"cors_origins"`
	// TrustedProxies 受信代理列表（IP 或 CIDR），用于正确获取客户端 IP。
	TrustedProxies []string `yaml:"trusted_proxies" json:"trusted_proxies"`
	// Health 健康检查端点配置。
	Health HealthConfig `yaml:"health" json:"health"`
}

// HealthConfig 健康检查端点路径和选项。
type HealthConfig struct {
	// LivePath 存活检查路径，默认 "/health/live"。
	LivePath string `yaml:"live_path" json:"live_path" default:"/health/live"`
	// ReadyPath 就绪检查路径，默认 "/health/ready"。
	ReadyPath string `yaml:"ready_path" json:"ready_path" default:"/health/ready"`
	// Detailed 返回详细健康信息（包含每个依赖的状态），默认 false。
	Detailed bool `yaml:"detailed" json:"detailed" default:"false"`
	// Timeout 每个检查项的超时时间（秒），默认 5 秒。
	Timeout int `yaml:"timeout" json:"timeout" default:"5"`
}

// ============================================================================
// HealthChecker — 健康检查器接口
// ============================================================================

// HealthChecker 是健康检查组件需要实现的接口。
// 用于 /health/ready 端点的依赖检查（DB、Redis、MQ 等）。
//
// 用法：
//
//	type DBChecker struct { DB *sql.DB }
//
//	func (c *DBChecker) Name() string { return "mysql" }
//	func (c *DBChecker) Check(ctx context.Context) error {
//	    return c.DB.PingContext(ctx)
//	}
//
//	svc.RegisterHealthChecker(&DBChecker{DB: db})
type HealthChecker interface {
	// Name 返回检查项名称，用于日志和响应中的标识。
	Name() string
	// Check 执行健康检查，返回 nil 表示健康，非 nil 表示不健康。
	Check(ctx context.Context) error
}

// HealthResult 单个检查项的结果。
type HealthResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`  // "ok" | "error"
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HealthReport 完整健康报告。
type HealthReport struct {
	Status   string         `json:"status"`  // "ok" | "degraded" | "error"
	Total    int            `json:"total"`
	Passed   int            `json:"passed"`
	Failed   int            `json:"failed"`
	Duration string         `json:"duration"`
	Checks   []HealthResult `json:"checks,omitempty"`
}

// ============================================================================
// 内置 HealthChecker 实现
// ============================================================================

// DBChecker 数据库健康检查器，支持 *sql.DB、GORM DB 和标准 database/sql
type DBChecker struct {
	DB interface {
		PingContext(ctx context.Context) error
	}
}

func (c *DBChecker) Name() string { return "database" }

func (c *DBChecker) Check(ctx context.Context) error {
	if c.DB == nil {
		return fmt.Errorf("database not configured")
	}
	return c.DB.PingContext(ctx)
}

// RedisChecker Redis 健康检查器，支持 *redis.Client、*redispool.Pool
type RedisChecker struct {
	Client interface {
		Ping(ctx context.Context) error
	}
}

func (c *RedisChecker) Name() string { return "redis" }

func (c *RedisChecker) Check(ctx context.Context) error {
	if c.Client == nil {
		return fmt.Errorf("redis not configured")
	}
	return c.Client.Ping(ctx)
}

// HTTPEndpointChecker HTTP 端点健康检查器
type HTTPEndpointChecker struct {
	NameValue string
	URL       string
	Client    *http.Client
	Expected  int
}

func (c *HTTPEndpointChecker) Name() string {
	if c.NameValue != "" {
		return c.NameValue
	}
	return c.URL
}

func (c *HTTPEndpointChecker) Check(ctx context.Context) error {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	expected := c.Expected
	if expected == 0 {
		expected = http.StatusOK
	}

	if resp.StatusCode != expected {
		return fmt.Errorf("unexpected status: %d (expected %d)", resp.StatusCode, expected)
	}
	return nil
}

// CustomCheckerFunc 适配函数为 HealthChecker
type CustomCheckerFunc struct {
	NameValue string
	CheckFn  func(ctx context.Context) error
}

func (f *CustomCheckerFunc) Name() string { return f.NameValue }
func (f *CustomCheckerFunc) Check(ctx context.Context) error { return f.CheckFn(ctx) }

// ============================================================================
// Options — 函数式配置
// ============================================================================

// Options 控制 boot.New 的行为。
type Options struct {
	cfg            *Config         // 编程注入的配置（优先级最高）
	logger         *slog.Logger    // 自定义 Logger
	configPath     string          // YAML 配置文件路径
	envPrefix      string          // 环境变量前缀
	extraSources   []config.Source // 额外配置源
	noConfig       bool            // true = 跳过配置加载，仅用默认值
	noHealth       bool            // true = 不注册 health 端点
	noDefaultMW    bool            // true = 不注册默认中间件（Recovery/RequestID）
	noLoggerInit   bool            // true = 跳过自动 Logger 初始化
	noConfigWatch  bool            // true = 禁用配置热重载
	backendName    string          // 强制后端名，空=按环境选择
	backendMapping map[string]string // 自定义 env→backend 映射
}

// Option 是 boot.New 的函数式参数。
type Option func(*Options)

func applyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

// WithConfigPath 设置 YAML 配置文件路径。
// 文件不存在则跳过，仅通过默认值 + 环境变量运行。
func WithConfigPath(path string) Option {
	return func(o *Options) { o.configPath = path }
}

// WithEnvPrefix 设置环境变量前缀，如 "USC"。
// 环境变量优先级高于配置文件，配置值按 __ 作为嵌套分隔符（如 USC__DB__PORT=5432）。
func WithEnvPrefix(prefix string) Option {
	return func(o *Options) { o.envPrefix = prefix }
}

// WithConfig 编程注入配置，覆盖文件和环境变量中的同名字段。
// 与文件/环境变量设计最大区别在于：零值字段不会覆盖目标值。
// 若要故意清空某字段，应在外部处理好后再通过此选项传入。
func WithConfig(cfg *Config) Option {
	return func(o *Options) { o.cfg = cfg }
}

// WithLogger 使用自定义 Logger 替代自动初始化。
func WithLogger(logger *slog.Logger) Option {
	return func(o *Options) { o.logger = logger }
}

// WithConfigSources 添加额外的配置数据源（例如 etcd、Nacos）。
func WithConfigSources(sources ...config.Source) Option {
	return func(o *Options) {
		o.extraSources = append(o.extraSources, sources...)
	}
}

// WithoutConfig 跳过配置加载，全部使用默认值（适用于纯测试）。
func WithoutConfig() Option {
	return func(o *Options) { o.noConfig = true }
}

// WithoutHealth 跳过健康检查端点注册。
func WithoutHealth() Option {
	return func(o *Options) { o.noHealth = true }
}

// WithoutDefaultMiddleware 跳过默认中间件（Recovery）注册。
func WithoutDefaultMiddleware() Option {
	return func(o *Options) { o.noDefaultMW = true }
}

// WithoutLoggerInit 跳过自动 Logger 初始化，使用 slog.Default()。
func WithoutLoggerInit() Option {
	return func(o *Options) { o.noLoggerInit = true }
}

// WithoutConfigWatch 禁止自动启动配置文件变更监听（禁用热重载）。
func WithoutConfigWatch() Option {
	return func(o *Options) { o.noConfigWatch = true }
}

// WithBackend 强制使用特定后端实现，忽略环境选择。
// 等价于显式配置 StorageBackend 字段，跳过 env→backend 映射。
//
//	svc := boot.New("order-svc", boot.WithBackend("sql-redis"))
//	repo := svc.Backend().MustSelect(cfg.Mode) // 总是返回 sql-redis
func WithBackend(name string) Option {
	return func(o *Options) { o.backendName = name }
}

// WithBackendMapping 设置自定义 env→backend 映射。
// 合并到默认映射（dev→memory, prod→sql-redis），相同 key 覆盖。
//
//	svc := boot.New("my-svc", boot.WithBackendMapping(map[string]string{
//	    "dev":     "sqlite",
//	    "ci":      "memory",
//	    "staging": "mysql",
//	}))
func WithBackendMapping(mapping map[string]string) Option {
	return func(o *Options) { o.backendMapping = mapping }
}

// ============================================================================
// Service — 启动脚手架核心
// ============================================================================

// Service 封装 Astra App、配置、日志和多环境后端选择器。
// 提供 Use / Router / Run 链式启动流程。
type Service struct {
	app     *astra.App
	cfg     *Config
	logger  *slog.Logger
	backend *backend.BackendSelector

	// 配置热重载支持
	mgr        *config.Config
	watchHooks []func()
	watchMu    sync.RWMutex
	watchOnce  sync.Once
	// Reloadable 组件注册
	reloadables []Reloadable

	// 健康检查器
	healthCheckers []HealthChecker
}

// New 创建 Service 实例。
// name 必填，用于日志、跟踪和配置默认值。
func New(name string, opts ...Option) *Service {
	o := applyOptions(opts...)

	// ---- 1. 配置加载 ----
	cfg := defaultConfig(name)
	var mgr *config.Config
	if !o.noConfig {
		loaded, m, err := loadConfigWithMgr(o)
		if err != nil {
			// 配置加载失败时使用默认值 + 警告（不阻止启动）
			slog.Warn("boot: config loading failed, using defaults",
				slog.String("service", name),
				slog.String("error", err.Error()),
			)
		} else if loaded != nil {
			cfg = loaded
			mgr = m
		}
	}
	// 编程注入的配置 > 文件/环境变量，逐字段覆盖
	if o.cfg != nil {
		overrideConfig(cfg, o.cfg)
	}

	// ---- 2. Logger 初始化 ----
	var logger *slog.Logger
	if o.logger != nil {
		logger = o.logger
	} else if o.noLoggerInit {
		logger = slog.Default()
	} else {
		logger = initLogger(cfg)
	}

	// ---- 3. Astra App 创建 ----
	appOpts := []astra.Option{
		astra.WithMode(astra.Mode(cfg.Mode)),
		astra.WithShutdownTimeout(cfg.ShutdownTimeout),
	}
	if len(cfg.TrustedProxies) > 0 {
		appOpts = append(appOpts, astra.WithTrustedProxies(cfg.TrustedProxies))
	}
	app := astra.New(appOpts...)

	if !o.noDefaultMW {
		app.Use(middleware.Recovery())
		app.Use(middleware.RequestID())
	}

	// ---- 4. 多环境后端选择器 ----
	var be *backend.BackendSelector
	if o.backendName != "" || len(o.backendMapping) > 0 {
		beOpts := make([]backend.Option, 0, 2)
		if o.backendName != "" {
			beOpts = append(beOpts, backend.WithBackend(o.backendName))
		}
		if len(o.backendMapping) > 0 {
			beOpts = append(beOpts, backend.WithMapping(o.backendMapping))
		}
		be = backend.New(name, beOpts...)
	}

	// ---- 5. Health 端点 ----
	// 注意：Health 端点注册在 New 返回 Service 后调用，以便访问 healthCheckers
	_ = app // 保存到 svc 后再注册

	svc := &Service{
		app:            app,
		cfg:            cfg,
		logger:         logger,
		backend:        be,
		mgr:            mgr,
		watchHooks:     make([]func(), 0),
		reloadables:    make([]Reloadable, 0),
		healthCheckers: make([]HealthChecker, 0),
	}

	// ---- 6. 配置热重载（默认启用）----
	if !o.noConfigWatch && mgr != nil {
		svc.startConfigWatch(context.Background())
	}

	// ---- 7. Health 端点注册 ----
	if !o.noHealth {
		svc.registerHealthEndpoints()
	}

	return svc
}

// ============================================================================
// Service 方法
// ============================================================================

// App 返回底层 Astra App 实例。
func (s *Service) App() *astra.App { return s.app }

// Backend 返回按环境选择实现后端的 Provider 选择器。
// 在 New 时通过 WithBackend / WithBackendMapping 配置。
// 未配置时返回 nil。
func (s *Service) Backend() *backend.BackendSelector { return s.backend }

// Cfg 返回运行时配置（只读，线程安全由 config.Config 保证）。
func (s *Service) Cfg() *Config { return s.cfg }

// Logger 返回运行时 Logger。
func (s *Service) Logger() *slog.Logger { return s.logger }

// Use 注册全局中间件。可多次调用，依次追加。
func (s *Service) Use(middleware ...astra.HandlerFunc) {
	s.app.Use(middleware...)
}

// Group 创建带可选中间件的路由组。
// 相当于 s.App().Group(prefix, middleware...)。
func (s *Service) Group(prefix string, middleware ...astra.HandlerFunc) *astra.Group {
	return s.app.Group(prefix, middleware...)
}

// UseLogger 注册日志中间件（自动跳过 health 端点路径）。
func (s *Service) UseLogger() {
	live := s.cfg.Health.LivePath
	ready := s.cfg.Health.ReadyPath
	skipPaths := make([]string, 0, 2)
	if live != "" {
		skipPaths = append(skipPaths, live)
	}
	if ready != "" {
		skipPaths = append(skipPaths, ready)
	}
	s.app.Use(
		middleware.LoggerWithConfig(middleware.LoggerConfig{
			Logger:    s.logger,
			SkipPaths: skipPaths,
		}),
	)
}

// UseCORS 注册 CORS 中间件（如果配置了 CORSOrigins）。
func (s *Service) UseCORS() {
	if len(s.cfg.CORSOrigins) > 0 {
		s.app.Use(middleware.CORS(s.cfg.CORSOrigins...))
	}
}

// Router 接收 App 实例，用于注册业务路由。
//
//	svc := boot.New("my-svc")
//	svc.Router(func(app *astra.App) {
//	    h := NewXxxHandler(svc)
//	    h.Register(app.Group("/api/v1"))
//	})
func (s *Service) Router(setup func(app *astra.App)) {
	setup(s.app)
}

// Run 启动 HTTP 服务并阻塞等待信号。
// 优雅关闭由 Astra 内置的 runWithGracefulShutdown 负责。
func (s *Service) Run() {
	addr := ":" + s.cfg.Port

	s.logger.Info("starting service",
		slog.String("name", s.cfg.Name),
		slog.String("addr", addr),
		slog.String("mode", s.cfg.Mode),
	)

	if err := s.app.Run(addr); err != nil {
		s.logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// ============================================================================
// Reloadable --- 热重载生命周期接口
// ============================================================================

// Reloadable 是组件可实现的接口，当配置热重载时，框架会自动调用
// RegisterReloadable 注册的所有组件的 Reload 方法。
//
// 适用于需要在运行时重新连接 DB/Redis、更新限流策略、切换日志级别的组件。
type Reloadable interface {
	// Reload 在配置热重载后被调用。
	// oldCfg 是之前的配置实例（可能为 nil 首次），newCfg 是新的配置实例。
	// 注意：newCfg 的生命周期由框架管理，不要持有引用，在回调内取完值即可。
	Reload(ctx context.Context, oldCfg, newCfg any) error
}

// RegisterReloadable 注册可热重载的组件。
// 当配置变更时，框架自动调用所有已注册组件的 Reload 方法。
// Reload 调用顺序与注册顺序一致。
// 如果组件的 Reload 返回 error，会记录日志但不会阻塞其他组件。
func (s *Service) RegisterReloadable(r Reloadable) {
	s.reloadables = append(s.reloadables, r)
}

// ============================================================================
// 健康检查
// ============================================================================

// RegisterHealthChecker 注册健康检查依赖（DB、Redis、MQ 等）。
// 这些检查项会在 /health/ready 端点被调用。
//
//	svc.RegisterHealthChecker(&boot.DBChecker{DB: db})
//	svc.RegisterHealthChecker(&boot.RedisChecker{Redis: redis})
//	svc.RegisterHealthChecker(&boot.HTTPEndpointChecker{Name: "user-svc", URL: "http://localhost:8081/health"})
func (s *Service) RegisterHealthChecker(checker HealthChecker) {
	s.healthCheckers = append(s.healthCheckers, checker)
}

// RegisterHealthCheckerFunc 注册函数形式的健康检查。
//
//	svc.RegisterHealthCheckerFunc("mysql", func(ctx context.Context) error {
//	    return db.PingContext(ctx)
//	})
func (s *Service) RegisterHealthCheckerFunc(name string, fn func(ctx context.Context) error) {
	s.healthCheckers = append(s.healthCheckers, &CustomCheckerFunc{
		NameValue: name,
		CheckFn:  fn,
	})
}

// CheckHealth 执行所有健康检查，返回完整报告。
func (s *Service) CheckHealth(ctx context.Context) *HealthReport {
	start := time.Now()
	report := &HealthReport{
		Total: len(s.healthCheckers),
		Checks: make([]HealthResult, 0, len(s.healthCheckers)),
	}

	for _, checker := range s.healthCheckers {
		result := HealthResult{Name: checker.Name()}
		checkStart := time.Now()

		// 设置超时
		timeout := time.Duration(s.cfg.Health.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := checker.Check(checkCtx); err != nil {
			result.Status = "error"
			result.Error = err.Error()
			report.Failed++
			s.logger.Debug("health check failed",
				slog.String("checker", checker.Name()),
				slog.String("error", err.Error()),
			)
		} else {
			result.Status = "ok"
			report.Passed++
		}

		result.Latency = time.Since(checkStart).String()
		report.Checks = append(report.Checks, result)
	}

	// 计算总体状态
	if report.Failed == 0 {
		report.Status = "ok"
	} else if report.Passed > 0 {
		report.Status = "degraded"
	} else {
		report.Status = "error"
	}

	report.Duration = time.Since(start).String()
	return report
}

// ============================================================================
// 配置热重载
// ============================================================================

// Watch 注册配置变更回调。回调在配置重新加载后调用（在单独 goroutine 中）。
// 多次调用会追加回调，回调顺序与注册顺序一致。
//
//	svc.Watch(func() {
//	    slog.Info("config reloaded", "qps", svc.Cfg().RateLimitQPS)
//	    // 重新初始化限流器、更新日志级别等
//	})
func (s *Service) Watch(fn func()) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	s.watchHooks = append(s.watchHooks, fn)
}

// WatchKey 注册键级配置变更回调，仅当指定键的值变化时触发。
// 等价于直接使用 s.mgr.WatchKey()，提供便捷访问。
//
//	svc.WatchKey("rate_limit.qps", func(oldVal, newVal string) {
//	    slog.Info("rate limit changed", "old", oldVal, "new", newVal)
//	})
func (s *Service) WatchKey(key string, fn func(oldVal, newVal string)) {
	if s.mgr == nil {
		return
	}
	s.mgr.WatchKey(key, fn)
}

// startConfigWatch 启动配置文件监听（内部方法，仅在 New 时调用一次）
func (s *Service) startConfigWatch(ctx context.Context) {
	s.watchOnce.Do(func() {
		if s.mgr == nil {
			return
		}

		// 注册全局回调：重新加载配置并通知所有 Watcher
		s.mgr.Watch(func() {
			// 重新 Scan 到新的 Config
			var newCfg Config
			if err := s.mgr.Scan(&newCfg); err != nil {
				s.logger.Error("boot: config reload scan failed", "error", err)
				return
			}

			s.watchMu.Lock()
			cfgBefore := s.cfg
			s.cfg = &newCfg
			hooks := make([]func(), len(s.watchHooks))
			copy(hooks, s.watchHooks)
			reloadables := make([]Reloadable, len(s.reloadables))
			copy(reloadables, s.reloadables)
			s.watchMu.Unlock()

			s.logger.Info("boot: config reloaded",
				slog.String("name", s.cfg.Name),
				slog.String("port", s.cfg.Port),
				slog.String("mode", s.cfg.Mode),
			)

			// 在单独 goroutine 中调用所有回调
			for _, fn := range hooks {
				go fn()
			}

			// 通知所有 Reloadable 组件
			for _, r := range reloadables {
				go func(rel Reloadable) {
					if err := rel.Reload(context.Background(), cfgBefore, &newCfg); err != nil {
						s.logger.Error("boot: component reload failed",
							slog.String("error", err.Error()),
						)
					}
				}(r)
			}
		})

		// 启动底层文件监听
		if err := s.mgr.StartWatch(ctx); err != nil {
			s.logger.Warn("boot: failed to start config watch", "error", err)
		}
	})
}

// StopConfigWatch 停止配置热重载监听（通常不需要手动调用，Run() 退出时会自动停止）
func (s *Service) StopConfigWatch() {
	if s.mgr != nil {
		s.mgr.StopWatch()
	}
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// defaultConfig 返回基于 name 的默认配置。
func defaultConfig(name string) *Config {
	return &Config{
		Name:            name,
		Port:            "8080",
		Mode:            string(astra.ModeDev),
		LogLevel:        "info",
		LogFormat:       "auto",
		ShutdownTimeout: 30,
		Health: HealthConfig{
			LivePath:  "/health/live",
			ReadyPath: "/health/ready",
		},
	}
}

// overrideConfig 用 src 覆盖 dst 的非零字段。
func overrideConfig(dst, src *Config) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Port != "" {
		dst.Port = src.Port
	}
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.LogLevel != "" {
		dst.LogLevel = src.LogLevel
	}
	if src.LogFormat != "" {
		dst.LogFormat = src.LogFormat
	}
	if src.ShutdownTimeout > 0 {
		dst.ShutdownTimeout = src.ShutdownTimeout
	}
	if len(src.CORSOrigins) > 0 {
		dst.CORSOrigins = src.CORSOrigins
	}
	if len(src.TrustedProxies) > 0 {
		dst.TrustedProxies = src.TrustedProxies
	}
	if src.Health.LivePath != "" {
		dst.Health.LivePath = src.Health.LivePath
	}
	if src.Health.ReadyPath != "" {
		dst.Health.ReadyPath = src.Health.ReadyPath
	}
}

// loadConfigWithMgr 构建配置管理器并加载到 Config 结构体。
// 返回配置实例和管理器（用于热重载），文件不存在时静默降级。
func loadConfigWithMgr(o *Options) (*Config, *config.Config, error) {
	sources := make([]config.Source, 0, 3)

	// 1. 配置文件（可选，不存在则跳过）
	if o.configPath != "" {
		if fi, err := os.Stat(o.configPath); err == nil && !fi.IsDir() {
			sources = append(sources, &config.YAMLFile{Path: o.configPath})
		}
	}

	// 2. 环境变量
	if o.envPrefix != "" {
		sources = append(sources, &config.Env{Prefix: o.envPrefix})
	}

	// 3. 额外数据源（远程配置中心等）
	sources = append(sources, o.extraSources...)

	// 4. 没有有效数据源时返回 nil，让调用方保持默认值
	if len(sources) == 0 {
		return nil, nil, nil
	}

	mgr, err := config.New(sources...)
	if err != nil {
		return nil, nil, err
	}

	var cfg Config
	if err := mgr.Scan(&cfg); err != nil {
		return nil, nil, fmt.Errorf("boot: scan config: %w", err)
	}

	return &cfg, mgr, nil
}

// initLogger 根据配置初始化 slog.Logger。
// dev → text handler，prod → JSON handler。
func initLogger(cfg *Config) *slog.Logger {
	level := parseLogLevel(cfg.LogLevel)
	format := cfg.LogFormat
	if format == "auto" {
		if cfg.Mode == "prod" || cfg.Mode == "staging" {
			format = "json"
		} else {
			format = "text"
		}
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler.WithGroup(cfg.Name))
}

// parseLogLevel 解析日志级别字符串。
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// registerHealthEndpoints 在 App 上注册健康检查端点。
// 使用闭包捕获 Service 实例以访问健康检查器。
func (s *Service) registerHealthEndpoints() {
	hc := &s.cfg.Health

	// Live 端点：简单存活检查
	if hc.LivePath != "" {
		s.app.GET(hc.LivePath, func(c *astra.Ctx) error {
			return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
		})
	}

	// Ready 端点：依赖检查
	if hc.ReadyPath != "" && hc.ReadyPath != hc.LivePath {
		s.app.GET(hc.ReadyPath, func(c *astra.Ctx) error {
			ctx := c.Request().Context()
			report := s.CheckHealth(ctx)

			// 根据配置决定返回内容
			if hc.Detailed || len(s.healthCheckers) == 0 {
				// 返回详细报告
				switch report.Status {
				case "ok":
					return c.JSON(http.StatusOK, report)
				case "degraded":
					return c.JSON(http.StatusOK, report) // 部分降级仍返回 200
				default:
					return c.JSON(http.StatusServiceUnavailable, report)
				}
			}

			// 简单模式：只返回状态
			switch report.Status {
			case "ok":
				return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
			case "degraded":
				return c.JSON(http.StatusOK, astra.Map{"status": "ok"}) // 兼容旧行为
			default:
				return c.JSON(http.StatusServiceUnavailable, astra.Map{"status": "error"})
			}
		})
	} else if hc.ReadyPath != "" {
		// Live 和 Ready 使用同一路径
		s.app.GET(hc.ReadyPath, func(c *astra.Ctx) error {
			ctx := c.Request().Context()
			report := s.CheckHealth(ctx)

			switch report.Status {
			case "ok":
				return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
			case "degraded":
				return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
			default:
				return c.JSON(http.StatusServiceUnavailable, astra.Map{"status": "error"})
			}
		})
	}
}
