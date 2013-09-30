package store

import (
	"container/list"
	"path"
	"strings"
	"sync/atomic"
)

type watcherHub struct {
	watchers     map[string]*list.List
	count        int64 // current number of watchers.
	EventHistory *EventHistory
}

type watcher struct {
	eventChan  chan *Event
	recursive  bool
	sinceIndex uint64
}

func newWatchHub(capacity int) *watcherHub {
	return &watcherHub{
		watchers:     make(map[string]*list.List),
		EventHistory: newEventHistory(capacity),
	}
}

// watch function returns an Event channel.
// If recursive is true, the first change after index under prefix will be sent to the event channel.
// If recursive is false, the first change after index at prefix will be sent to the event channel.
// If index is zero, watch will start from the current index + 1.
func (wh *watcherHub) watch(prefix string, recursive bool, index uint64) (<-chan *Event, error) {
	eventChan := make(chan *Event, 1)

	e, err := wh.EventHistory.scan(prefix, index)

	if err != nil {
		return nil, err
	}

	if e != nil {
		eventChan <- e
		return eventChan, nil
	}

	w := &watcher{
		eventChan:  eventChan,
		recursive:  recursive,
		sinceIndex: index,
	}

	l, ok := wh.watchers[prefix]

	if ok { // add the new watcher to the back of the list
		l.PushBack(w)

	} else { // create a new list and add the new watcher
		l := list.New()
		l.PushBack(w)
		wh.watchers[prefix] = l
	}

	atomic.AddInt64(&wh.count, 1)

	return eventChan, nil
}

func (wh *watcherHub) notifyWithPath(e *Event, path string, force bool) {
	l, ok := wh.watchers[path]

	if ok {
		curr := l.Front()
		notifiedAll := true

		for {

			if curr == nil { // we have reached the end of the list

				if notifiedAll {
					// if we have notified all watcher in the list
					// we can delete the list
					delete(wh.watchers, path)
				}
				break
			}

			next := curr.Next() // save the next

			w, _ := curr.Value.(*watcher)

			if (w.recursive || force || e.Key == path) && e.Index >= w.sinceIndex {
				w.eventChan <- e
				l.Remove(curr)
				atomic.AddInt64(&wh.count, -1)
			} else {
				notifiedAll = false
			}

			curr = next // go to the next one

		}
	}
}

func (wh *watcherHub) notify(e *Event) {
	segments := strings.Split(e.Key, "/")

	currPath := "/"

	// walk through all the paths
	for _, segment := range segments {
		currPath = path.Join(currPath, segment)
		wh.notifyWithPath(e, currPath, false)
	}

	wh.EventHistory.addEvent(e)
}

// notify with last event's index and term
func (wh *watcherHub) notifyWithoutIndex(action, key string) {
	e := wh.EventHistory.addEventWithouIndex(action, key)

	segments := strings.Split(e.Key, "/")

	currPath := "/"

	// walk through all the paths
	for _, segment := range segments {
		currPath = path.Join(currPath, segment)
		wh.notifyWithPath(e, currPath, false)
	}
}

func (wh *watcherHub) clone() *watcherHub {
	clonedHistory := wh.EventHistory.clone()

	return &watcherHub{
		EventHistory: clonedHistory,
	}
}
