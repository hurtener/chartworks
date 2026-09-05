package gateway

import "time"

// Check validates ownership even when a cached result consumes no new provider attempt.
func(b *Budget)Check(call Call)error{
 if b==nil||!call.Valid(){return ErrBudget};b.mu.Lock();defer b.mu.Unlock()
 if call.Key()!=b.key||!time.Now().Before(b.deadline)||b.tokens>b.maxTokens{return ErrBudget};return nil
}
// Observe conservatively charges a reported overage; reservations are never refunded on
// unknown usage or failures. A provider overage cannot fund another retry in this operation.
func(b *Budget)Observe(call Call,reserved,reported int)error{
 if b==nil||!call.Valid(){return ErrBudget};b.mu.Lock();defer b.mu.Unlock()
 if call.Key()!=b.key||reported<0||reserved<1{return ErrBudget}
 if reported>reserved{extra:=reported-reserved;if extra>b.maxTokens-b.tokens{b.tokens=b.maxTokens+1;return ErrBudget};b.tokens+=extra}
 return nil
}
