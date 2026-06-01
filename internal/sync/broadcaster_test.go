package sync

import (
	"testing"
)

func TestBroadcasterDeliversToSubscriber(t *testing.T) {
	b := NewBroadcaster()
	ch, cancel := b.Subscribe(7)
	defer cancel()

	b.publish(7, Event{RepoID: 7, Phase: "commits", Message: "page 1", Done: false})

	select {
	case ev := <-ch:
		if ev.RepoID != 7 || ev.Phase != "commits" || ev.Message != "page 1" {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("expected an event to be delivered")
	}
}

func TestBroadcasterIsolatesRepos(t *testing.T) {
	b := NewBroadcaster()
	ch7, cancel7 := b.Subscribe(7)
	defer cancel7()
	ch8, cancel8 := b.Subscribe(8)
	defer cancel8()

	b.publish(8, Event{RepoID: 8, Phase: "done", Done: true})

	select {
	case <-ch7:
		t.Fatal("repo 7 subscriber should not see repo 8 events")
	default:
	}
	select {
	case ev := <-ch8:
		if !ev.Done {
			t.Fatalf("expected done event, got %+v", ev)
		}
	default:
		t.Fatal("repo 8 subscriber missed its event")
	}
}

func TestBroadcasterCancelUnsubscribes(t *testing.T) {
	b := NewBroadcaster()
	ch, cancel := b.Subscribe(7)
	cancel()
	// Publishing after cancel must not panic (closed/removed channel) and the
	// channel is closed so a receive returns the zero value with ok=false.
	b.publish(7, Event{RepoID: 7, Phase: "commits"})
	if _, ok := <-ch; ok {
		t.Fatal("expected channel closed after cancel")
	}
}

func TestBroadcasterDoesNotBlockWhenBufferFull(t *testing.T) {
	b := NewBroadcaster()
	_, cancel := b.Subscribe(7)
	defer cancel()
	// Flood far beyond the buffer; publish must never block.
	for i := 0; i < 10000; i++ {
		b.publish(7, Event{RepoID: 7, Phase: "commits", Message: "x"})
	}
}
