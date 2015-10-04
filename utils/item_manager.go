package utils

import (
	"github.com/petar/GoLLRB/llrb"
	"sync"
	"time"
	"math/rand"
	"encoding/json"
	"fmt"
	"bytes"
	"compress/gzip"
	"math"
)


var (
	max_items 		int = 100
	mill_hour		float64 = 1000*60*60
	mill_day			float64 = mill_hour*24
)

type IManager struct {
	sortedItems		*llrb.LLRB
	itemsByUrl		map[string]*Item
	itemsById		map[int64]*Item
	mutex			*sync.RWMutex
	json				string
	json_zipped		[]byte
}

//this corresponds to an image (a piece of news) shown in the feeds page
type Item struct {
	Img_url 			string		`json:"img_url,omitempty"`
	Id				int64		`json:"id,omitempty"`
	Title			string		`json:"title,omitempty"`
	Ncomments		int			`json:"n_comm"`
	BestComment		*Comment		`json:"b_comm,omitempty"`
	time				int64		/*`json:"-"`*/	
	score 			int64
}

func NewManager() *IManager {
	m := &IManager{}
	m.sortedItems = llrb.New()
	m.itemsByUrl = make(map[string]*Item)
	m.itemsById = make(map[int64]*Item)
	m.mutex = &sync.RWMutex{}
	return m
}

func (item Item) Less (than llrb.Item) bool {
	it := than.(*Item)
	return item.score < it.score
}

func (item *Item) updateScore() {
	result := item.time + int64(math.Sqrt(float64(item.Ncomments))*mill_day/2)
	if item.BestComment != nil {
		result += int64(math.Sqrt(float64(item.BestComment.Likes))*mill_day/2)
	}
	item.score = result
}

func (m *IManager) NotifyComment (comment *Comment) {
	//could add a check here to see if item is managed only with a read lock
	m.mutex.Lock()
	defer m.mutex.Unlock()	
	item, ok := m.itemsById[comment.Item_id]
	if !ok {
		//the item is no longer managed, so don't bother updating the list
		return
	}
	//only update the best comment if it is null or has zero likes
	m.sortedItems.Delete(item)
	prev_comm := item.BestComment
	if prev_comm == nil || prev_comm.Likes == 0 {
		item.BestComment = comment
	}
	item.Ncomments = item.Ncomments + 1
	item.updateScore()
	m.sortedItems.InsertNoReplace(item)
	m.refreshJson()	
}

func (m *IManager) NotifyItemId (url string, id int64) {
	m.mutex.Lock()	
	defer m.mutex.Unlock()
	item, ok := m.itemsByUrl[url]
	if !ok {
		return
	}
	item.Id = id
	m.itemsById[id] = item
}


func (m *IManager) IsManaged (item *Item) bool {
	m.mutex.RLock()	
	defer m.mutex.RUnlock()
	_, ok := m.itemsByUrl[item.Img_url]
	if !ok {
		_, ok = m.itemsById[item.Id]
	}
	return ok
}

func (m *IManager) GetItemByUrl (img_url string) (*Item, bool) {
	m.mutex.RLock()	
	defer m.mutex.RUnlock()
	item, ok := m.itemsByUrl[img_url]
	return item, ok
}


func (m *IManager) Insert (item *Item) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	_, ok := m.itemsByUrl[item.Img_url]
	if ok {return false}
	//set timestamp partly randomly to shuffle items
	item.time = time.Now().UnixNano()/1000000 + rand.Int63n(1000)
	item.updateScore()
	m.sortedItems.InsertNoReplace(item)
	m.itemsByUrl[item.Img_url] = item
	m.removeTail()
	return true
}

func (m *IManager) RefreshJson(){	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.refreshJson()
}

func (m *IManager) refreshJson(){	
	l := len(m.itemsByUrl)
	var ary = make([]*Item, l)	
	m.sortedItems.AscendGreaterOrEqual(&Item{}, func(i llrb.Item) bool {
		//ary = append(ary, i.(*Item))
		ary[l-1] = i.(*Item)
		l--
		return true
	})
	b, err := json.Marshal(ary)
    if err != nil {
        fmt.Println(err)
        return
    }
	m.json = string(b)
	buf := new(bytes.Buffer)
	w := gzip.NewWriter(buf)
	w.Write([]byte(m.json))
	w.Flush()
	m.json_zipped = buf.Bytes()
}

//to be called inside a lock
func (m *IManager) removeTail(){
	l := len(m.itemsByUrl)
	if l <= max_items {return}	
	for i := 0; i < l-max_items; i++ {
		min := m.sortedItems.DeleteMin().(*Item)
		delete(m.itemsByUrl, min.Img_url)
		delete(m.itemsById, min.Id)
	}
	if (m.sortedItems.Len() != len(m.itemsByUrl)){
		panic("manager lists have different lengths!!")
	}
}

func (m *IManager) GetJson() string{
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.json
}

func (m *IManager) GetZippedJson() []byte{
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.json_zipped
}