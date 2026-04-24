package budget

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_ZeroForNonPositiveDuration(t *testing.T) {
	assert.Equal(t, int64(0), EstimateTokens("opus", 0))
	assert.Equal(t, int64(0), EstimateTokens("opus", -30))
}

func TestEstimateTokens_OneMinuteMatchesHeuristic(t *testing.T) {
	// 50K input + 5K output = 55K total per minute per doc.go.
	got := EstimateTokens("opus", 60)
	assert.Equal(t, int64(55_000), got)
}

func TestEstimateTokens_ModelAgnostic(t *testing.T) {
	// The heuristic itself does not depend on model — token counts are the
	// same for opus and sonnet. Only cost differs.
	opus := EstimateTokens("opus", 600)
	sonnet := EstimateTokens("sonnet", 600)
	assert.Equal(t, opus, sonnet)
}

func TestEstimateCost_OpusRates(t *testing.T) {
	// 1M input @ $5/M + 1M output @ $25/M = $30.
	got := EstimateCost("opus", 1_000_000, 1_000_000)
	assert.InDelta(t, 30.0, got, 0.001)
}

func TestEstimateCost_SonnetRates(t *testing.T) {
	// 1M input @ $3/M + 1M output @ $15/M = $18.
	got := EstimateCost("sonnet", 1_000_000, 1_000_000)
	assert.InDelta(t, 18.0, got, 0.001)
}

func TestEstimateCost_UnknownModelFallsBackToSonnet(t *testing.T) {
	// Any non-opus model bills at Sonnet rates so operators get a
	// conservative estimate.
	unknown := EstimateCost("haiku-ish-future-model", 1_000_000, 1_000_000)
	sonnet := EstimateCost("sonnet", 1_000_000, 1_000_000)
	assert.Equal(t, sonnet, unknown)
}

func TestEstimateCost_ZeroTokensZeroCost(t *testing.T) {
	assert.Equal(t, 0.0, EstimateCost("opus", 0, 0))
}

func TestTracker_RecordSessionAccumulates(t *testing.T) {
	tr := NewTracker(0)
	tr.RecordSession("opus", 60)
	first := tr.GetTotalCost()

	tr.RecordSession("opus", 60)
	second := tr.GetTotalCost()

	// Two one-minute opus sessions cost twice one one-minute session.
	assert.InDelta(t, first*2, second, 0.0001)
}

func TestTracker_RecordSessionOpusOneMinute(t *testing.T) {
	// Sanity check on the headline cost figure used in demos.
	// Opus: 50K input @ $5/M + 5K output @ $25/M = $0.25 + $0.125 = $0.375 per minute.
	tr := NewTracker(0)
	tr.RecordSession("opus", 60)
	assert.InDelta(t, 0.375, tr.GetTotalCost(), 0.001)
}

func TestTracker_RecordSessionSonnetOneMinute(t *testing.T) {
	// Sonnet: 50K input @ $3/M + 5K output @ $15/M = $0.15 + $0.075 = $0.225 per minute.
	tr := NewTracker(0)
	tr.RecordSession("sonnet", 60)
	assert.InDelta(t, 0.225, tr.GetTotalCost(), 0.001)
}

func TestTracker_RecordSessionNonPositiveIsNoOp(t *testing.T) {
	tr := NewTracker(0)
	tr.RecordSession("opus", 0)
	tr.RecordSession("opus", -10)
	assert.Equal(t, 0.0, tr.GetTotalCost())
}

func TestTracker_IsOverBudget_UnderLimit(t *testing.T) {
	tr := NewTracker(100.0)
	tr.RecordSession("opus", 60) // $0.375
	assert.False(t, tr.IsOverBudget())
}

func TestTracker_IsOverBudget_EqualsLimit(t *testing.T) {
	// Boundary: exactly at the limit is over-budget (>=), matching the
	// existing isBudgetExceeded semantics in the reconciler.
	tr := NewTracker(0.375)
	tr.RecordSession("opus", 60)
	assert.True(t, tr.IsOverBudget())
}

func TestTracker_IsOverBudget_AboveLimit(t *testing.T) {
	tr := NewTracker(0.10)
	tr.RecordSession("opus", 60) // $0.375 > $0.10
	assert.True(t, tr.IsOverBudget())
}

func TestTracker_IsOverBudget_NoLimitMeansNeverOver(t *testing.T) {
	// Zero (or negative) budget limit disables the check — without this,
	// teams declared without a budget would always trip the over-budget
	// path on the first cost above zero.
	tr := NewTracker(0)
	tr.RecordSession("opus", 6000) // $37.50 accumulated
	assert.False(t, tr.IsOverBudget())

	tr2 := NewTracker(-1)
	tr2.RecordSession("opus", 60)
	assert.False(t, tr2.IsOverBudget())
}

func TestTracker_OpusMoreExpensiveThanSonnet(t *testing.T) {
	opus := NewTracker(0)
	sonnet := NewTracker(0)
	opus.RecordSession("opus", 600)
	sonnet.RecordSession("sonnet", 600)
	assert.Greater(t, opus.GetTotalCost(), sonnet.GetTotalCost())
}

func TestTracker_ConcurrentRecordSession(t *testing.T) {
	// Tracker is documented as safe for concurrent use; race detector would
	// catch missing locks.
	tr := NewTracker(0)
	const (
		goroutines = 50
		perG       = 10
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				tr.RecordSession("opus", 60)
			}
		}()
	}
	wg.Wait()

	// Each call adds $0.375; 500 calls total.
	expected := 0.375 * float64(goroutines*perG)
	assert.InDelta(t, expected, tr.GetTotalCost(), 0.01)
}
