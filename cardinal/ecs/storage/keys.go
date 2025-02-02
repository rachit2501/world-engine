package storage

/*
	NONCE STORAGE:      ADDRESS_TO_NONCE -> Nonce used for verifying signatures.
	Hash set of signature address to uint64 nonce
*/

func (r *RedisStorage) nonceKey() string {
	return "ADDRESS_TO_NONCE"
}
