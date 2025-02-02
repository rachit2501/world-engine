package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrSystemTransactionRequired  = errors.New("system transaction required")
	ErrSystemTransactionForbidden = errors.New("system transaction forbidden")
)

func decode[T any](buf []byte) (T, error) {
	dec := json.NewDecoder(bytes.NewBuffer(buf))
	dec.DisallowUnknownFields()
	var val T
	if err := dec.Decode(&val); err != nil {
		return val, err
	}
	return val, nil
}

func getSignerAddressFromPayload(sp sign.Transaction) (string, error) {
	createPersonaTx, err := decode[ecs.CreatePersonaTransaction](sp.Body)
	if err != nil {
		return "", err
	}
	return createPersonaTx.SignerAddress, nil
}

func (handler *Handler) verifySignature(sp *sign.Transaction, isSystemTransaction bool,
) (sig *sign.Transaction, err error) {
	if sp.PersonaTag == "" {
		return nil, errors.New("PersonaTag must not be empty")
	}

	// Handle the case where signature is disabled
	if handler.disableSigVerification {
		return sp, nil
	}
	///////////////////////////////////////////////

	// Check that the namespace is correct
	if sp.Namespace != handler.w.Namespace().String() {
		return nil, fmt.Errorf("%w: got namespace %q but it must be %q",
			ErrInvalidSignature, sp.Namespace, handler.w.Namespace().String())
	}
	if isSystemTransaction && !sp.IsSystemTransaction() {
		return nil, ErrSystemTransactionRequired
	} else if !isSystemTransaction && sp.IsSystemTransaction() {
		return nil, ErrSystemTransactionForbidden
	}

	var signerAddress string
	if sp.IsSystemTransaction() {
		// For system transactions, just use the signed address that is include in the signature.
		signerAddress, err = getSignerAddressFromPayload(*sp)
	} else {
		// For non-system transaction, get the signer address from storage. If this PersonaTag doesn't exist,
		// an error will be returned and the signature verification will fail.
		signerAddress, err = handler.w.GetSignerForPersonaTag(sp.PersonaTag, 0)
	}
	if err != nil {
		return nil, err
	}

	// Check the nonce
	nonce, err := handler.w.GetNonce(signerAddress)
	if err != nil {
		return nil, err
	}
	if sp.Nonce <= nonce {
		return nil, fmt.Errorf("%w: got nonce %d, but must be greater than %d",
			ErrInvalidSignature, sp.Nonce, nonce)
	}

	// Verify signature
	if err = sp.Verify(signerAddress); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}
	// Update nonce
	if err = handler.w.SetNonce(signerAddress, sp.Nonce); err != nil {
		return nil, err
	}
	return sp, nil
}

func (handler *Handler) verifySignatureOfMapRequest(request map[string]interface{}, isSystemTransaction bool,
) (payload []byte, sig *sign.Transaction, err error) {
	sp, err := sign.MappedTransaction(request)
	if err != nil {
		return nil, nil, err
	}
	sig, err = handler.verifySignature(sp, isSystemTransaction)
	if err != nil {
		return nil, nil, err
	}
	if len(sp.Body) == 0 {
		buf, err := json.Marshal(request)
		if err != nil {
			return nil, nil, err
		}
		return buf, sp, nil
	}

	return sig.Body, sig, nil
}
