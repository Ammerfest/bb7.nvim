package orders

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// OrderStatus represents the state of an order.
type OrderStatus string

const (
	OrderPending   OrderStatus = "pending"
	OrderConfirmed OrderStatus = "confirmed"
	OrderShipped   OrderStatus = "shipped"
)

// Order holds information about a customer order.
type Order struct {
	ID        string
	Customer  string
	Items     []string
	Total     float64
	Status    OrderStatus
	CreatedAt time.Time
}

// OrderService manages order processing.
type OrderService struct {
	orders map[string]*Order
	nextID int
}

// NewOrderService creates a new OrderService.
func NewOrderService() *OrderService {
	return &OrderService{
		orders: make(map[string]*Order),
		nextID: 1,
	}
}

// CreateOrder creates a new order with the given items.
func (s *OrderService) CreateOrder(customer string, items []string, total float64) (*Order, error) {
	if customer == "" {
		return nil, fmt.Errorf("customer name is required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("at least one item is required")
	}
	if total <= 0 {
		return nil, fmt.Errorf("total must be positive")
	}

	order := &Order{
		ID:        fmt.Sprintf("ORD-%03d", s.nextID),
		Customer:  customer,
		Items:     items,
		Total:     total,
		Status:    OrderPending,
		CreatedAt: time.Now(),
	}
	s.nextID++
	s.orders[order.ID] = order

	log.Printf("created order %s for %s: %s (total: $%.2f)",
		order.ID, order.Customer, strings.Join(order.Items, ", "), order.Total)

	return order, nil
}

// GetOrder returns an order by ID.
func (s *OrderService) GetOrder(id string) (*Order, error) {
	order, ok := s.orders[id]
	if !ok {
		return nil, fmt.Errorf("order %s not found", id)
	}
	return order, nil
}

// ConfirmOrder changes an order's status to confirmed.
func (s *OrderService) ConfirmOrder(id string) error {
	order, err := s.GetOrder(id)
	if err != nil {
		return err
	}
	if order.Status != OrderPending {
		return fmt.Errorf("order %s is not pending (status: %s)", id, order.Status)
	}
	order.Status = OrderConfirmed
	log.Printf("confirmed order %s", id)
	return nil
}
