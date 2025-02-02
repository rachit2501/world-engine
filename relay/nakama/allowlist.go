package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/heroiclabs/nakama-common/runtime"
	"os"
	"strconv"
	"strings"
)

var (
	ErrNotAllowlisted     = errors.New("this user is not allowlisted")
	ErrInvalidBetaKey     = errors.New("invalid beta key")
	ErrBetaKeyAlreadyUsed = errors.New("beta key already used")
	ErrAlreadyVerified    = errors.New("this user is already verified by an existing beta key")

	allowlistEnabledEnvVar = "ENABLE_ALLOWLIST"
	allowlistEnabled       = false
	allowlistKeyCollection = "allowlist_keys_collection"
	allowedUsers           = "allowed_users"
)

func initAllowlist(_ runtime.Logger, initializer runtime.Initializer) error {
	enabledStr := os.Getenv(allowlistEnabledEnvVar)
	if enabledStr == "" {
		return nil
	}
	var err error
	allowlistEnabled, err = strconv.ParseBool(enabledStr)
	if err != nil {
		return fmt.Errorf("the ENABLE_ALLOWLIST flag was set, however the variable %q was an invalid "+
			"boolean: %w", enabledStr, err)
	}

	if !allowlistEnabled {
		return nil
	}
	err = initializer.RegisterRpc("generate-beta-keys", allowListRPC)
	if err != nil {
		return err
	}

	err = initializer.RegisterRpc("claim-key", claimKeyRPC)
	if err != nil {
		return err
	}
	return nil
}

type GenKeysMsg struct {
	Amount int `json:"amount"`
}

type GenKeysResponse struct {
	Keys []string `json:"keys"`
}

type KeyStorage struct {
	Key    string
	UsedBy string
	Used   bool
}

func allowListRPC(ctx context.Context, _ runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	id, err := getUserID(ctx)
	if err != nil {
		return "", err
	}
	if id != adminAccountID {
		return "", fmt.Errorf("unauthorized: only admin may call this RPC")
	}

	var msg GenKeysMsg
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return "", fmt.Errorf(`error unmarshaling payload: expected form {"amount": <int>}: %w`, err)
	}

	keys, err := generateBetaKeys(msg.Amount)
	if err != nil {
		return "", fmt.Errorf("error generating beta keys: %w", err)
	}

	writes := make([]*runtime.StorageWrite, 0, len(keys))
	for _, key := range keys {
		obj := KeyStorage{
			Key:    key,
			UsedBy: "",
			Used:   false,
		}
		bz, err := json.Marshal(obj)
		if err != nil {
			return "", err
		}
		writes = append(writes, &runtime.StorageWrite{
			Collection:      allowlistKeyCollection,
			Key:             key,
			UserID:          id,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		})
	}

	_, err = nk.StorageWrite(ctx, writes)
	if err != nil {
		return "", fmt.Errorf("error writing keys to storage: %w", err)
	}

	response, err := json.Marshal(GenKeysResponse{Keys: keys})
	if err != nil {
		return "", err
	}
	return string(response), nil
}

type ClaimKeyMsg struct {
	Key string `json:"key"`
}

type ClaimKeyRes struct {
	Success bool `json:"success"`
}

func claimKeyRPC(ctx context.Context, _ runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	userID, err := getUserID(ctx)
	if err != nil {
		return "", err
	}

	// if this user is already verified,
	err = checkVerified(ctx, nk, userID)
	if err == nil {
		return "", ErrAlreadyVerified
	}

	var ck ClaimKeyMsg
	err = json.Unmarshal([]byte(payload), &ck)
	if err != nil {
		return "", err
	}
	if ck.Key == "" {
		return "", fmt.Errorf("no beta key specified in request")
	}
	err = claimKey(ctx, nk, ck.Key, userID)
	if err != nil {
		return "", err
	}
	err = writeVerified(ctx, nk, userID)
	if err != nil {
		return "", err
	}

	bz, err := json.Marshal(ClaimKeyRes{Success: true})
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func writeVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
	type verified struct {
	}
	bz, err := json.Marshal(verified{})
	if err != nil {
		return err
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      allowedUsers,
			Key:             userID,
			UserID:          adminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	return err
}

func checkVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
	if !allowlistEnabled {
		return nil
	}
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: allowedUsers,
			Key:        userID,
			UserID:     adminAccountID,
		},
	})
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return ErrNotAllowlisted
	}
	return nil
}

func readKey(ctx context.Context, nk runtime.NakamaModule, key string) (*KeyStorage, error) {
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: allowlistKeyCollection,
			Key:        key,
			UserID:     adminAccountID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error reading storage object for key: %w", err)
	}
	if len(objs) == 0 {
		return nil, ErrInvalidBetaKey
	}

	obj := objs[0]
	var ks KeyStorage
	err = json.Unmarshal([]byte(obj.Value), &ks)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal storage object into %T: %w", ks, err)
	}
	return &ks, nil
}

func writeKey(ctx context.Context, nk runtime.NakamaModule, ks *KeyStorage) error {
	bz, err := json.Marshal(ks)
	if err != nil {
		return fmt.Errorf("could not marshal KeyStorage object: %w", err)
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      allowlistKeyCollection,
			Key:             ks.Key,
			UserID:          adminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	if err != nil {
		return fmt.Errorf("could not write KeyObject back to storage: %w", err)
	}
	return nil
}

func claimKey(ctx context.Context, nk runtime.NakamaModule, key, userID string) error {
	ks, err := readKey(ctx, nk, key)
	if err != nil {
		return err
	}
	if ks.Used {
		return ErrBetaKeyAlreadyUsed
	}
	ks.Used = true
	ks.UsedBy = userID

	err = writeKey(ctx, nk, ks)
	if err != nil {
		return err
	}

	return nil
}

func generateRandomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func generateBetaKeys(n int) ([]string, error) {
	const bzLen = 16
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		randomBytes, err := generateRandomBytes(bzLen) // 16 bytes for the desired format
		if err != nil {
			return nil, err
		}
		// Format the random bytes as a hyphen-separated string
		key := hex.EncodeToString(randomBytes)
		key = strings.ToUpper(key)
		key = fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
		keys = append(keys, key)
	}

	return keys, nil
}
