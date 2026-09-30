package chartworks

import "github.com/hurtener/chartworks/internal/nlq/generationdecision"

// GenerationChoice is a service-reviewed catalog reference offered alongside a
// durable generation question. Submit its kind/ID through NLQPlanRequest's
// References, with GenerationQuery and GenerationContext from the same problem.
// Use "resume:" + problem.QueryID as both PlanNLQ and RunNLQ operation.
type GenerationChoice = generationdecision.Choice
