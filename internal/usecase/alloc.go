package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"id-maker/internal/entity"
	"id-maker/pkg/snowflake"
)

type Alloc struct {
	Mu        sync.RWMutex
	BizTagMap map[string]*BizAlloc
}

type BizAlloc struct {
	Mu      sync.Mutex
	BazTag  string
	IdArray []*IdArray // 最多2段，0段标识当前正在发号的号段；1段后台提前从DB预期
	GetDb   bool       //当前正在查询DB
}

type IdArray struct {
	Cur   int64 //当前发到哪个位置
	Start int64 //最小值
	End   int64 //最大值
}

func (uc *SegmentUseCase) NewAllocId() (a *Alloc, err error) {
	var res []entity.Segments
	if res, err = uc.repo.GetList(); err != nil {
		return
	}
	a = &Alloc{
		BizTagMap: make(map[string]*BizAlloc),
	}
	for _, v := range res {
		a.BizTagMap[v.BizTag] = &BizAlloc{
			BazTag:  v.BizTag,
			GetDb:   false,
			IdArray: make([]*IdArray, 0),
		}
	}
	return
}

func (uc *SegmentUseCase) NewAllocSnowFlakeId() (*snowflake.Worker, error) {
	return snowflake.NewWorker(1)
}

func (b *BizAlloc) GetId(uc *SegmentUseCase) (id int64, err error) {
	var (
		canGetId    bool
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
	)
	b.Mu.Lock()
	// 先尝试内存拿号
	if b.LeftIdCount() > 0 {
		// LeftIdCount统计两段中全部的剩余号
		id = b.PopId() // 可能移除了1个段
		canGetId = true
	}
	//分配ID数组不足开始携程去申请新的ID
	if len(b.IdArray) <= 1 && !b.GetDb {
		// 在加锁范围内设置共享变量，记录当前有协程去操作数据库
		// 然后释放锁，去执行数据库操作，避免在锁中执行DB操作
		b.GetDb = true
		b.Mu.Unlock() // 先解锁再起协程，因为goroutine中也要获取这把锁
		go b.GetIdArray(cancel, uc)
	} else {
		b.Mu.Unlock()
		defer cancel()
	}
	if canGetId {
		return
	}
	// 当前协程等待去数据库操作完成
	select {
	case <-ctx.Done(): //执行结束或者超时
	}
	b.Mu.Lock()
	// 先快路径拿号；拿不到就等补货；补货后再拿一次
	if b.LeftIdCount() > 0 {
		id = b.PopId()
	} else {
		err = errors.New("no get id")
	}
	b.Mu.Unlock()
	return
}

func (b *BizAlloc) GetIdArray(cancel context.CancelFunc, uc *SegmentUseCase) {
	var (
		tryNum int
		ids    *entity.Segments
		err    error
	)
	// 完成数据库操作后，通知主协程
	defer cancel()
	for {
		if tryNum >= 3 { //失败重试 3 次
			b.Mu.Lock()
			// 修改共享状态加锁
			b.GetDb = false
			b.Mu.Unlock()
			break
		}
		b.Mu.Lock()
		if len(b.IdArray) <= 1 {
			b.Mu.Unlock()
			ids, err = uc.repo.GetNextId(b.BazTag)
			if err != nil {
				tryNum++
			} else {
				tryNum = 0
				b.Mu.Lock()
				b.IdArray = append(b.IdArray, &IdArray{Start: ids.MaxId, End: ids.MaxId + ids.Step})
				if len(b.IdArray) > 1 {
					b.GetDb = false
					b.Mu.Unlock()
					break
				} else {
					b.Mu.Unlock()
				}
			}
		} else {
			b.Mu.Unlock()
		}
	}
}

func (b *BizAlloc) LeftIdCount() (count int64) {
	for _, v := range b.IdArray {
		arr := v
		//结束位置-开始位置-已经分配的次数
		count += arr.End - arr.Start - arr.Cur
	}
	return count
}

func (b *BizAlloc) PopId() (id int64) {
	id = b.IdArray[0].Start + b.IdArray[0].Cur //开始位置加上分配次数
	b.IdArray[0].Cur++                         //分配次数 +1
	if id+1 >= b.IdArray[0].End {              //该数组里面没有ID了
		b.IdArray = append(b.IdArray[:0], b.IdArray[1:]...) //把分配完的数组移除
		// 把1位置后的数据写入索引0开始
	}
	return
}

func (b *BizAlloc) GetBatchId(uc *SegmentUseCase, num int64) (ids []int64, err error) {
	var (
		canGetId    bool
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
	)
	b.Mu.Lock()
	// 先尝试内存拿号
	if b.LeftIdCount() > num {
		// LeftIdCount统计两段中全部的剩余号
		ids = b.PopIds(num) // 可能移除了1个段
		canGetId = true
	}
	//分配ID数组不足开始携程去申请新的ID
	if len(b.IdArray) <= 1 && !b.GetDb {
		// 在加锁范围内设置共享变量，记录当前有协程去操作数据库
		// 然后释放锁，去执行数据库操作，避免在锁中执行DB操作
		b.GetDb = true
		b.Mu.Unlock() // 先解锁再起协程，因为goroutine中也要获取这把锁
		go b.GetIdArray(cancel, uc)
	} else {
		b.Mu.Unlock()
		defer cancel()
	}
	if canGetId {
		return
	}
	// 当前协程等待去数据库操作完成
	select {
	case <-ctx.Done(): //执行结束或者超时
	}
	b.Mu.Lock()
	// 先快路径拿号；拿不到就等补货；补货后再拿一次
	if b.LeftIdCount() > 0 {
		ids = b.PopIds(num)
	} else {
		err = errors.New("no get id")
	}
	b.Mu.Unlock()
	return
}

func (b *BizAlloc) PopIds(num int64) (ids []int64) {
	id := b.IdArray[0].Start + b.IdArray[0].Cur //开始位置加上分配次数
	b.IdArray[0].Cur = b.IdArray[0].Cur + num   //分配次数 + num
	if id+num >= b.IdArray[0].End {             //该数组里面没有ID了
		b.IdArray = append(b.IdArray[:0], b.IdArray[1:]...) //把分配完的数组移除
		// 把1位置后的数据写入索引0开始
	}
	ids = make([]int64, num)
	for i := range ids {
		ids[i] = id + int64(i)
	}
	return
}
