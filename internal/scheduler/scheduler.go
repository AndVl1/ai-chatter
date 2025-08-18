package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler управляет запланированными задачами
type Scheduler struct {
	cron       *cron.Cron
	ctx        context.Context
	cancel     context.CancelFunc
	reportFunc func(ctx context.Context) error
}

// New создает новый планировщик
func New() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		cron:   cron.New(cron.WithLocation(time.UTC)),
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetReportFunction устанавливает функцию для генерации отчетов
func (s *Scheduler) SetReportFunction(f func(ctx context.Context) error) {
	s.reportFunc = f
}

// Start запускает планировщик
func (s *Scheduler) Start() error {
	if s.reportFunc == nil {
		log.Println("⚠️ Report function not set, scheduler will not generate reports")
		return nil
	}

	// Ежедневно в 21:00 UTC
	_, err := s.cron.AddFunc("0 21 * * *", func() {
		log.Println("🕘 Triggered daily report generation at 21:00 UTC")
		if err := s.reportFunc(s.ctx); err != nil {
			log.Printf("❌ Daily report generation failed: %v", err)
		}
	})

	if err != nil {
		return err
	}

	s.cron.Start()
	log.Println("📅 Scheduler started - daily reports will be generated at 21:00 UTC")
	return nil
}

// Stop останавливает планировщик
func (s *Scheduler) Stop() {
	if s.cron != nil {
		ctx := s.cron.Stop()
		<-ctx.Done()
	}
	if s.cancel != nil {
		s.cancel()
	}
	log.Println("📅 Scheduler stopped")
}

// IsRunning проверяет, запущен ли планировщик
func (s *Scheduler) IsRunning() bool {
	return s.cron != nil && len(s.cron.Entries()) > 0
}
