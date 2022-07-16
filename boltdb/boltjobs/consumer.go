package boltjobs

import (
	"bytes"
	"context"
	"encoding/gob"
	"os"
	"sync"
	"sync/atomic"
	"time"

	cfgPlugin "github.com/js361014/api/v2/plugins/config"
	"github.com/js361014/api/v2/plugins/jobs"
	"github.com/js361014/api/v2/plugins/jobs/pipeline"
	priorityqueue "github.com/js361014/roadrunner/v2/priority_queue"
	"github.com/js361014/roadrunner/v2/utils"
	"github.com/spiral/errors"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

const (
	PluginName string = "boltdb"
	rrDB       string = "rr.db"

	PushBucket    string = "push"
	InQueueBucket string = "processing"
	DelayBucket   string = "delayed"
)

type consumer struct {
	file        string
	permissions int
	priority    int64
	prefetch    int

	db *bolt.DB

	bPool    sync.Pool
	log      *zap.Logger
	pq       priorityqueue.Queue
	pipeline atomic.Value
	cond     *sync.Cond

	listeners uint32
	active    *uint64
	delayed   *uint64

	stopCh chan struct{}
}

func NewBoltDBJobs(configKey string, log *zap.Logger, cfg cfgPlugin.Configurer, pq priorityqueue.Queue) (*consumer, error) {
	const op = errors.Op("init_boltdb_jobs")

	if !cfg.Has(configKey) {
		return nil, errors.E(op, errors.Errorf("no configuration by provided key: %s", configKey))
	}

	// if no global section
	if !cfg.Has(PluginName) {
		return nil, errors.E(op, errors.Str("no global boltdb configuration"))
	}

	var localCfg config
	err := cfg.UnmarshalKey(PluginName, &localCfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = cfg.UnmarshalKey(configKey, &localCfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	localCfg.InitDefaults()
	db, err := bolt.Open(localCfg.File, os.FileMode(localCfg.Permissions), &bolt.Options{
		Timeout:        time.Second * 20,
		NoGrowSync:     false,
		NoFreelistSync: false,
		ReadOnly:       false,
		NoSync:         false,
	})

	if err != nil {
		return nil, errors.E(op, err)
	}

	// create bucket if it does not exist
	// tx.Commit invokes via the db.Update
	err = db.Update(func(tx *bolt.Tx) error {
		const upOp = errors.Op("boltdb_plugin_update")
		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(DelayBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(PushBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(InQueueBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		inQb := tx.Bucket(utils.AsBytes(InQueueBucket))
		cursor := inQb.Cursor()

		pushB := tx.Bucket(utils.AsBytes(PushBucket))

		// get all items, which are in the InQueueBucket and put them into the PushBucket
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			err = pushB.Put(k, v)
			if err != nil {
				return errors.E(op, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, errors.E(op, err)
	}

	return &consumer{
		permissions: localCfg.Permissions,
		file:        localCfg.File,
		priority:    localCfg.Priority,
		prefetch:    localCfg.Prefetch,

		bPool: sync.Pool{New: func() interface{} {
			return new(bytes.Buffer)
		}},
		cond: sync.NewCond(&sync.Mutex{}),

		delayed: utils.Uint64(0),
		active:  utils.Uint64(0),

		db:     db,
		log:    log,
		pq:     pq,
		stopCh: make(chan struct{}),
	}, nil
}

func FromPipeline(pipeline *pipeline.Pipeline, log *zap.Logger, cfg cfgPlugin.Configurer, pq priorityqueue.Queue) (*consumer, error) {
	const op = errors.Op("init_boltdb_jobs")

	// if no global section
	if !cfg.Has(PluginName) {
		return nil, errors.E(op, errors.Str("no global boltdb configuration"))
	}

	var conf config
	err := cfg.UnmarshalKey(PluginName, conf)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// add default values
	conf.InitDefaults()

	db, err := bolt.Open(pipeline.String(file, rrDB), os.FileMode(conf.Permissions), &bolt.Options{
		Timeout:        time.Second * 20,
		NoGrowSync:     false,
		NoFreelistSync: false,
		ReadOnly:       false,
		NoSync:         false,
	})

	if err != nil {
		return nil, errors.E(op, err)
	}

	// create bucket if it does not exist
	// tx.Commit invokes via the db.Update
	err = db.Update(func(tx *bolt.Tx) error {
		const upOp = errors.Op("boltdb_plugin_update")
		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(DelayBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(PushBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		_, err = tx.CreateBucketIfNotExists(utils.AsBytes(InQueueBucket))
		if err != nil {
			return errors.E(op, upOp)
		}

		inQb := tx.Bucket(utils.AsBytes(InQueueBucket))
		cursor := inQb.Cursor()

		pushB := tx.Bucket(utils.AsBytes(PushBucket))

		// get all items, which are in the InQueueBucket and put them into the PushBucket
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			err = pushB.Put(k, v)
			if err != nil {
				return errors.E(op, err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, errors.E(op, err)
	}

	return &consumer{
		file:        pipeline.String(file, rrDB),
		priority:    pipeline.Priority(),
		prefetch:    pipeline.Int(prefetch, 1000),
		permissions: conf.Permissions,

		bPool: sync.Pool{New: func() interface{} {
			return new(bytes.Buffer)
		}},
		cond: sync.NewCond(&sync.Mutex{}),

		delayed: utils.Uint64(0),
		active:  utils.Uint64(0),

		db:     db,
		log:    log,
		pq:     pq,
		stopCh: make(chan struct{}),
	}, nil
}

func (c *consumer) Push(_ context.Context, job *jobs.Job) error {
	const op = errors.Op("boltdb_jobs_push")
	err := c.db.Update(func(tx *bolt.Tx) error {
		item := fromJob(job)
		// pool with buffers
		buf := c.get()
		// encode the job
		enc := gob.NewEncoder(buf)
		err := enc.Encode(item)
		if err != nil {
			c.put(buf)
			return errors.E(op, err)
		}

		value := make([]byte, buf.Len())
		copy(value, buf.Bytes())
		c.put(buf)

		// handle delay
		if item.Options.Delay > 0 {
			b := tx.Bucket(utils.AsBytes(DelayBucket))
			tKey := time.Now().UTC().Add(time.Second * time.Duration(item.Options.Delay)).Format(time.RFC3339)

			err = b.Put(utils.AsBytes(tKey), value)
			if err != nil {
				return errors.E(op, err)
			}

			atomic.AddUint64(c.delayed, 1)

			return nil
		}

		b := tx.Bucket(utils.AsBytes(PushBucket))
		err = b.Put(utils.AsBytes(item.ID()), value)
		if err != nil {
			return errors.E(op, err)
		}

		// increment active counter
		atomic.AddUint64(c.active, 1)

		return nil
	})

	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (c *consumer) Register(_ context.Context, pipeline *pipeline.Pipeline) error {
	c.pipeline.Store(pipeline)
	return nil
}

func (c *consumer) Run(_ context.Context, p *pipeline.Pipeline) error {
	const op = errors.Op("boltdb_run")
	start := time.Now()

	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p.Name() {
		return errors.E(op, errors.Errorf("no such pipeline registered: %s", pipe.Name()))
	}

	// run listener
	go c.listener()
	go c.delayedJobsListener()

	// increase number of listeners
	atomic.AddUint32(&c.listeners, 1)
	c.log.Debug("pipeline is active", zap.String("driver", pipe.Driver()), zap.String("pipeline", pipe.Name()), zap.Time("start", start), zap.Duration("elapsed", time.Since(start)))
	return nil
}

func (c *consumer) Stop(_ context.Context) error {
	start := time.Now()
	if atomic.LoadUint32(&c.listeners) > 0 {
		c.stopCh <- struct{}{}
		c.stopCh <- struct{}{}
	}

	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	c.log.Debug("pipeline was stopped", zap.String("driver", pipe.Driver()), zap.String("pipeline", pipe.Name()), zap.Time("start", start), zap.Duration("elapsed", time.Since(start)))
	return c.db.Close()
}

func (c *consumer) Pause(_ context.Context, p string) {
	start := time.Now()
	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p {
		c.log.Error("no such pipeline", zap.String("pause was requested", p))
	}

	l := atomic.LoadUint32(&c.listeners)
	// no active listeners
	if l == 0 {
		c.log.Warn("no active listeners, nothing to pause")
		return
	}

	c.stopCh <- struct{}{}
	c.stopCh <- struct{}{}

	atomic.AddUint32(&c.listeners, ^uint32(0))

	c.log.Debug("pipeline was paused", zap.String("driver", pipe.Driver()), zap.String("pipeline", pipe.Name()), zap.Time("start", start), zap.Duration("elapsed", time.Since(start)))
}

func (c *consumer) Resume(_ context.Context, p string) {
	start := time.Now()
	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p {
		c.log.Error("no such pipeline", zap.String("resume was requested", p))
	}

	l := atomic.LoadUint32(&c.listeners)
	// no active listeners
	if l == 1 {
		c.log.Warn("amqp listener already in the active state")
		return
	}

	// run listener
	go c.listener()
	go c.delayedJobsListener()

	// increase number of listeners
	atomic.AddUint32(&c.listeners, 1)

	c.log.Debug("pipeline was resumed", zap.String("driver", pipe.Driver()), zap.String("pipeline", pipe.Name()), zap.Time("start", start), zap.Duration("elapsed", time.Since(start)))
}

func (c *consumer) State(_ context.Context) (*jobs.State, error) {
	pipe := c.pipeline.Load().(*pipeline.Pipeline)

	return &jobs.State{
		Pipeline: pipe.Name(),
		Driver:   pipe.Driver(),
		Queue:    PushBucket,
		Active:   int64(atomic.LoadUint64(c.active)),
		Delayed:  int64(atomic.LoadUint64(c.delayed)),
		Ready:    toBool(atomic.LoadUint32(&c.listeners)),
	}, nil
}

// Private

func (c *consumer) get() *bytes.Buffer {
	return c.bPool.Get().(*bytes.Buffer)
}

func (c *consumer) put(b *bytes.Buffer) {
	b.Reset()
	c.bPool.Put(b)
}

func toBool(r uint32) bool {
	return r > 0
}
