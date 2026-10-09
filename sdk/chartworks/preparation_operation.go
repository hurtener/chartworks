package chartworks

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
)

// PreparationOperationVersion is explicitly set only on new Prepare requests.
const PreparationOperationVersion = reporting.AuthoringPreparationOperationVersion

// NewPreparationOperation allocates one explicit Prepare intent. Keep this key
// and the exact body for recovery; never replace it automatically after timeout,
// migration-required, expiry or a missing retained result.
func NewPreparationOperation(now time.Time) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "prepare:" + strconv.FormatInt(now.Unix(), 10) + ":" + hex.EncodeToString(nonce[:]), nil
}
