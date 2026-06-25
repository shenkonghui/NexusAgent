package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// SchedulerExecutor 是调度器执行定时任务所需的 agent 能力子集（*agent.Router 实现该接口）。
type SchedulerExecutor interface {
	CreateSessionWithSource(ctx context.Context, agentType, cwd string, userID uint, source string) (*models.Session, error)
	PromptWithExecution(ctx context.Context, sessionID, prompt string, executionID *uint) (<-chan models.Message, error)
	ResumeSession(ctx context.Context, sessionID, cwdOverride string) (*models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ListExecutions(sessionID string) ([]repository.ExecutionAggregate, error)
}

// SchedulerService 是进程内 cron 调度器，管理定时任务的调度与执行。
type SchedulerService struct {
	repo *repository.ScheduledTaskRepository
	exec SchedulerExecutor
	cron *cron.Cron

	mu       sync.Mutex
	entries  map[uint]cron.EntryID // taskID -> cron entry ID
	taskLock sync.Map              // taskID -> *sync.Mutex（per-task 执行互斥）
	stopOnce sync.Once
}

// NewSchedulerService 创建调度器。调用 Start() 后开始调度。
func NewSchedulerService(repo *repository.ScheduledTaskRepository, exec SchedulerExecutor) *SchedulerService {
	return &SchedulerService{
		repo:    repo,
		exec:    exec,
		cron:    cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		entries: make(map[uint]cron.EntryID),
	}
}

// Start 启动调度器并加载所有 enabled 任务。
func (s *SchedulerService) Start() error {
	tasks, err := s.repo.FindAllEnabled()
	if err != nil {
		return fmt.Errorf("加载定时任务失败: %w", err)
	}
	for i := range tasks {
		t := tasks[i]
		if err := s.schedule(&t); err != nil {
			log.Printf("调度定时任务 %d (%s) 失败: %v", t.ID, t.Name, err)
		}
	}
	s.cron.Start()
	log.Printf("定时任务调度器已启动，共加载 %d 个任务", len(tasks))
	return nil
}

// Stop 停止调度器。
func (s *SchedulerService) Stop() {
	s.stopOnce.Do(func() {
		ctx := s.cron.Stop()
		<-ctx.Done()
	})
}

// schedule 为单个任务添加 cron 调度项。
func (s *SchedulerService) schedule(t *models.ScheduledTask) error {
	entryID, err := s.cron.AddFunc(t.CronExpr, func() {
		s.run(t.ID)
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[t.ID] = entryID
	s.mu.Unlock()
	return nil
}

// unschedule 移除任务的 cron 调度项。
func (s *SchedulerService) unschedule(taskID uint) {
	s.mu.Lock()
	entryID, ok := s.entries[taskID]
	if ok {
		delete(s.entries, taskID)
	}
	s.mu.Unlock()
	if ok {
		s.cron.Remove(entryID)
	}
}

// AddTask 创建任务后注册调度。
func (s *SchedulerService) AddTask(t *models.ScheduledTask) error {
	if err := s.repo.Create(t); err != nil {
		return err
	}
	if t.Enabled {
		if err := s.schedule(t); err != nil {
			return fmt.Errorf("任务已创建但调度失败: %w", err)
		}
	}
	return nil
}

// UpdateTask 更新任务并重新调度。
func (s *SchedulerService) UpdateTask(t *models.ScheduledTask) error {
	if err := s.repo.Update(t); err != nil {
		return err
	}
	s.unschedule(t.ID)
	if t.Enabled {
		if err := s.schedule(t); err != nil {
			return fmt.Errorf("任务已更新但调度失败: %w", err)
		}
	}
	return nil
}

// RemoveTask 删除任务及其关联会话，并移除调度。
func (s *SchedulerService) RemoveTask(taskID uint) error {
	t, err := s.repo.FindByID(taskID)
	if err != nil {
		return err
	}
	s.unschedule(taskID)
	// 删除关联会话（若有）
	if t.SessionID != "" {
		_ = s.exec.DeleteSession(context.Background(), t.SessionID)
	}
	return s.repo.Delete(taskID)
}

// RunTask 手动触发一次执行。
func (s *SchedulerService) RunTask(taskID uint) error {
	t, err := s.repo.FindByID(taskID)
	if err != nil {
		return err
	}
	go s.run(t.ID)
	return nil
}

// run 执行一次定时任务。包含 per-task 互斥（重叠跳过）、session 准备、prompt 执行。
func (s *SchedulerService) run(taskID uint) {
	t, err := s.repo.FindByID(taskID)
	if err != nil {
		log.Printf("定时任务 %d 不存在: %v", taskID, err)
		return
	}

	// per-task 互斥：非阻塞，重叠则跳过
	mu, _ := s.taskLock.LoadOrStore(taskID, &sync.Mutex{})
	taskMu := mu.(*sync.Mutex)
	if !taskMu.TryLock() {
		_ = s.repo.UpdateRunResult(taskID, models.TaskStatusSkipped, "上一次执行尚未结束", time.Now())
		log.Printf("定时任务 %d (%s) 跳过：上一次执行尚未结束", taskID, t.Name)
		return
	}
	defer taskMu.Unlock()

	_ = s.repo.UpdateRunResult(taskID, models.TaskStatusRunning, "", time.Now())

	if err := s.execute(t); err != nil {
		_ = s.repo.UpdateRunResult(taskID, models.TaskStatusFailed, err.Error(), time.Now())
		log.Printf("定时任务 %d (%s) 执行失败: %v", taskID, t.Name, err)
		return
	}
	_ = s.repo.UpdateRunResult(taskID, models.TaskStatusSuccess, "", time.Now())
}

// execute 准备 session 并发送 prompt，同步消费消息流至结束。
func (s *SchedulerService) execute(t *models.ScheduledTask) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("执行 panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	session, execID, err := s.ensureSession(ctx, t)
	if err != nil {
		return err
	}

	ch, err := s.exec.PromptWithExecution(ctx, session.SessionID, t.Prompt, &execID)
	if err != nil {
		return fmt.Errorf("发送 prompt: %w", err)
	}

	// 同步消费消息流至结束
	for range ch {
	}
	return nil
}

// ensureSession 确保任务关联的 session 处于活跃可用状态，返回 session 与本次 execution_id。
// 首次执行时创建 session 并回填任务；session 已 closed/error 时恢复。
func (s *SchedulerService) ensureSession(ctx context.Context, t *models.ScheduledTask) (*models.Session, uint, error) {
	var session *models.Session
	var err error

	if t.DBSessionID == 0 {
		// 首次执行：创建 session
		session, err = s.exec.CreateSessionWithSource(ctx, t.AgentType, t.Cwd, t.UserID, models.SessionSourceScheduled)
		if err != nil {
			return nil, 0, fmt.Errorf("创建会话: %w", err)
		}
		if err := s.repo.UpdateSessionRef(t.ID, session.SessionID, session.ID); err != nil {
			return nil, 0, fmt.Errorf("回填会话引用: %w", err)
		}
	} else {
		session, err = s.exec.GetSessionByDBID(t.DBSessionID)
		if err != nil {
			return nil, 0, fmt.Errorf("查询关联会话: %w", err)
		}
		// session 已关闭或出错则恢复
		if session.Status != models.SessionStatusActive {
			session, err = s.exec.ResumeSession(ctx, session.SessionID, t.Cwd)
			if err != nil {
				return nil, 0, fmt.Errorf("恢复会话: %w", err)
			}
		}
	}

	// 计算本次 execution_id（当前最大 + 1）
	// 通过 ListExecutions 获取聚合，取第一条（最新）的 execution_id + 1
	execs, err := s.exec.ListExecutions(session.SessionID)
	if err != nil {
		return nil, 0, fmt.Errorf("查询执行历史: %w", err)
	}
	var execID uint = 1
	if len(execs) > 0 {
		execID = execs[0].ExecutionID + 1
	}
	return session, execID, nil
}
