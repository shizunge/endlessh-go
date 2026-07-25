package metrics

import (
	"testing"
	"time"
)

func TestUpdatablePriorityQueue_PushAndPop(t *testing.T) {
	pq := NewUpdatablePriorityQueue()

	now := time.Now()
	pq.Update("item1", now.Add(2*time.Second))
	pq.Update("item2", now.Add(1*time.Second))
	pq.Update("item3", now.Add(3*time.Second))

	if pq.pq.Len() != 3 {
		t.Fatalf("Expected length 3, got %d", pq.pq.Len())
	}

	first := pq.Pop()
	if first == nil {
		t.Fatal("Pop returned nil")
	}
	if first.Key != "item2" {
		t.Errorf("Expected first to be item2, got %s", first.Key)
	}

	second := pq.Pop()
	if second.Key != "item1" {
		t.Errorf("Expected second to be item1, got %s", second.Key)
	}

	third := pq.Pop()
	if third.Key != "item3" {
		t.Errorf("Expected third to be item3, got %s", third.Key)
	}

	if pq.Pop() != nil {
		t.Error("Expected pop on empty pq to return nil")
	}
}

func TestUpdatablePriorityQueue_UpdateExisting(t *testing.T) {
	pq := NewUpdatablePriorityQueue()

	now := time.Now()
	pq.Update("item1", now.Add(2*time.Second))
	pq.Update("item2", now.Add(3*time.Second))

	// Update item2 to have an earlier time
	pq.Update("item2", now.Add(1*time.Second))

	first := pq.Peek()
	if first == nil {
		t.Fatal("Peek returned nil")
	}
	if first.Key != "item2" {
		t.Errorf("Expected first to be item2 after update, got %s", first.Key)
	}

	popFirst := pq.Pop()
	if popFirst.Key != "item2" {
		t.Errorf("Expected popped first to be item2, got %s", popFirst.Key)
	}

	popSecond := pq.Pop()
	if popSecond.Key != "item1" {
		t.Errorf("Expected popped second to be item1, got %s", popSecond.Key)
	}
}

func TestUpdatablePriorityQueue_UpdateToLater(t *testing.T) {
	pq := NewUpdatablePriorityQueue()

	now := time.Now()
	pq.Update("item1", now.Add(1*time.Second))
	pq.Update("item2", now.Add(2*time.Second))

	// Update item1 to have a later time than item2
	pq.Update("item1", now.Add(3*time.Second))

	first := pq.Pop()
	if first == nil || first.Key != "item2" {
		t.Errorf("Expected first to be item2, got %v", first)
	}

	second := pq.Pop()
	if second == nil || second.Key != "item1" {
		t.Errorf("Expected second to be item1, got %v", second)
	}
}

func TestUpdatablePriorityQueue_Empty(t *testing.T) {
	pq := NewUpdatablePriorityQueue()

	if pq.Peek() != nil {
		t.Error("Expected Peek on empty pq to return nil")
	}
	if pq.Pop() != nil {
		t.Error("Expected Pop on empty pq to return nil")
	}
}
