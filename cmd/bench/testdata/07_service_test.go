package orders

import (
	"strings"
	"testing"
)

func TestCreateOrder(t *testing.T) {
	svc := NewOrderService()

	order, err := svc.CreateOrder("Alice", []string{"Widget", "Gadget"}, 29.99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.ID != "ORD-001" {
		t.Errorf("got ID %s, want ORD-001", order.ID)
	}
	if order.Customer != "Alice" {
		t.Errorf("got customer %q, want Alice", order.Customer)
	}
	if order.Status != OrderPending {
		t.Errorf("got status %q, want pending", order.Status)
	}
}

func TestCreateOrder_EmptyCustomer(t *testing.T) {
	svc := NewOrderService()
	_, err := svc.CreateOrder("", []string{"Widget"}, 10.00)
	if err == nil {
		t.Fatal("expected error for empty customer")
	}
	if !strings.Contains(err.Error(), "customer") {
		t.Errorf("error %q should mention customer", err.Error())
	}
}

func TestCreateOrder_NoItems(t *testing.T) {
	svc := NewOrderService()
	_, err := svc.CreateOrder("Bob", nil, 10.00)
	if err == nil {
		t.Fatal("expected error for no items")
	}
}

func TestGetOrder(t *testing.T) {
	svc := NewOrderService()
	created, _ := svc.CreateOrder("Charlie", []string{"Doohickey"}, 15.00)

	got, err := svc.GetOrder(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Customer != "Charlie" {
		t.Errorf("got customer %q, want Charlie", got.Customer)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	svc := NewOrderService()
	_, err := svc.GetOrder("ORD-999")
	if err == nil {
		t.Fatal("expected error for missing order")
	}
}

func TestConfirmOrder(t *testing.T) {
	svc := NewOrderService()
	order, _ := svc.CreateOrder("Diana", []string{"Thingamajig"}, 42.00)

	err := svc.ConfirmOrder(order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.GetOrder(order.ID)
	if got.Status != OrderConfirmed {
		t.Errorf("got status %q, want confirmed", got.Status)
	}
}

func TestConfirmOrder_NotPending(t *testing.T) {
	svc := NewOrderService()
	order, _ := svc.CreateOrder("Eve", []string{"Widget"}, 10.00)
	_ = svc.ConfirmOrder(order.ID) // now confirmed

	err := svc.ConfirmOrder(order.ID) // try again
	if err == nil {
		t.Fatal("expected error for non-pending order")
	}
}
