// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package waddrmgr

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/dcrlabs/bchwallet/internal/zero"
	"github.com/dcrlabs/bchwallet/walletdb"
	"github.com/gcash/bchd/bchec"
	"github.com/gcash/bchutil"
	"github.com/gcash/bchutil/hdkeychain"
)

// AddressType represents the various address types waddrmgr is currently able
// to generate, and maintain.
//
// NOTE: These MUST be stable as they're used for scope address schema
// recognition within the database.
type AddressType uint8

const (
	// PubKeyHash is a regular p2pkh address.
	PubKeyHash AddressType = iota

	// Script reprints a raw script address.
	Script

	// RawPubKey is just raw public key to be used within scripts, This
	// type indicates that a scoped manager with this address type
	// shouldn't be consulted during historical rescans.
	RawPubKey
)

// ManagedAddress is an interface that provides acces to information regarding
// an address managed by an address manager. Concrete implementations of this
// type may provide further fields to provide information specific to that type
// of address.
type ManagedAddress interface {
	// Account returns the account the address is associated with.
	Account() uint32

	// Address returns a bchutil.Address for the backing address.
	Address() bchutil.Address

	// AddrHash returns the key or script hash related to the address
	AddrHash() []byte

	// Imported returns true if the backing address was imported instead
	// of being part of an address chain.
	Imported() bool

	// Internal returns true if the backing address was created for internal
	// use such as a change output of a transaction.
	Internal() bool

	// Compressed returns true if the backing address is compressed.
	Compressed() bool

	// Used returns true if the backing address has been used in a transaction.
	Used(ns walletdb.ReadBucket) bool

	// AddrType returns the address type of the managed address. This can
	// be used to quickly discern the address type without further
	// processing
	AddrType() AddressType
}

// ManagedPubKeyAddress extends ManagedAddress and additionally provides the
// public and private keys for pubkey-based addresses.
type ManagedPubKeyAddress interface {
	ManagedAddress

	// PubKey returns the public key associated with the address.
	PubKey() *bchec.PublicKey

	// ExportPubKey returns the public key associated with the address
	// serialized as a hex encoded string.
	ExportPubKey() string

	// PrivKey returns the private key for the address.  It can fail if the
	// address manager is watching-only or locked, or the address does not
	// have any keys.
	PrivKey() (*bchec.PrivateKey, error)

	// ExportPrivKey returns the private key associated with the address
	// serialized as Wallet Import Format (WIF).
	ExportPrivKey() (*bchutil.WIF, error)

	// DerivationInfo contains the information required to derive the key
	// that backs the address via traditional methods from the HD root. For
	// imported keys, the first value will be set to false to indicate that
	// we don't know exactly how the key was derived.
	DerivationInfo() (KeyScope, DerivationPath, bool)
}

// ManagedScriptAddress extends ManagedAddress and represents a pay-to-script-hash
// style of bitcoin addresses.  It additionally provides information about the
// script.
type ManagedScriptAddress interface {
	ManagedAddress

	// Script returns the script associated with the address.
	Script() ([]byte, error)
}

// managedAddress represents a public key address.  It also may or may not have
// the private key associated with the public key.
type managedAddress struct {
	manager          *ScopedKeyManager
	derivationPath   DerivationPath
	address          bchutil.Address
	imported         bool
	internal         bool
	compressed       bool
	used             bool
	addrType         AddressType
	pubKey           *bchec.PublicKey
	privKeyEncrypted []byte
	privKeyCT        []byte // non-nil if unlocked
	privKeyMutex     sync.Mutex
}

// Enforce managedAddress satisfies the ManagedPubKeyAddress interface.
var _ ManagedPubKeyAddress = (*managedAddress)(nil)

// unlock decrypts and stores a pointer to the associated private key.  It will
// fail if the key is invalid or the encrypted private key is not available.
// The returned clear text private key will always be a copy that may be safely
// used by the caller without worrying about it being zeroed during an address
// lock.
func (a *managedAddress) unlock(key EncryptorDecryptor) ([]byte, error) {
	// Protect concurrent access to clear text private key.
	a.privKeyMutex.Lock()
	defer a.privKeyMutex.Unlock()

	if len(a.privKeyCT) == 0 {
		privKey, err := key.Decrypt(a.privKeyEncrypted)
		if err != nil {
			str := fmt.Sprintf("failed to decrypt private key for "+
				"%s", a.address)
			return nil, managerError(ErrCrypto, str, err)
		}

		a.privKeyCT = privKey
	}

	privKeyCopy := make([]byte, len(a.privKeyCT))
	copy(privKeyCopy, a.privKeyCT)
	return privKeyCopy, nil
}

// lock zeroes the associated clear text private key.
func (a *managedAddress) lock() {
	// Zero and nil the clear text private key associated with this
	// address.
	a.privKeyMutex.Lock()
	zero.Bytes(a.privKeyCT)
	a.privKeyCT = nil
	a.privKeyMutex.Unlock()
}

// Account returns the account number the address is associated with.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Account() uint32 {
	return a.derivationPath.Account
}

// AddrType returns the address type of the managed address. This can be used
// to quickly discern the address type without further processing
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) AddrType() AddressType {
	return a.addrType
}

// Address returns the bchutil.Address which represents the managed address.
// This will be a pay-to-pubkey-hash address.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Address() bchutil.Address {
	return a.address
}

// AddrHash returns the public key hash for the address.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) AddrHash() []byte {
	var hash []byte

	switch n := a.address.(type) {
	case *bchutil.AddressPubKeyHash:
		hash = n.Hash160()[:]
	case *bchutil.AddressScriptHash:
		hash = n.Hash160()[:]
	}

	return hash
}

// Imported returns true if the address was imported instead of being part of an
// address chain.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Imported() bool {
	return a.imported
}

// Internal returns true if the address was created for internal use such as a
// change output of a transaction.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Internal() bool {
	return a.internal
}

// Compressed returns true if the address is compressed.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Compressed() bool {
	return a.compressed
}

// Used returns true if the address has been used in a transaction.
//
// This is part of the ManagedAddress interface implementation.
func (a *managedAddress) Used(ns walletdb.ReadBucket) bool {
	return a.manager.fetchUsed(ns, a.AddrHash())
}

// PubKey returns the public key associated with the address.
//
// This is part of the ManagedPubKeyAddress interface implementation.
func (a *managedAddress) PubKey() *bchec.PublicKey {
	return a.pubKey
}

// pubKeyBytes returns the serialized public key bytes for the managed address
// based on whether or not the managed address is marked as compressed.
func (a *managedAddress) pubKeyBytes() []byte {
	if a.compressed {
		return a.pubKey.SerializeCompressed()
	}
	return a.pubKey.SerializeUncompressed()
}

// ExportPubKey returns the public key associated with the address
// serialized as a hex encoded string.
//
// This is part of the ManagedPubKeyAddress interface implementation.
func (a *managedAddress) ExportPubKey() string {
	return hex.EncodeToString(a.pubKeyBytes())
}

// PrivKey returns the private key for the address.  It can fail if the address
// manager is watching-only or locked, or the address does not have any keys.
//
// This is part of the ManagedPubKeyAddress interface implementation.
func (a *managedAddress) PrivKey() (*bchec.PrivateKey, error) {
	// No private keys are available for a watching-only address manager.
	if a.manager.rootManager.WatchOnly() {
		return nil, managerError(ErrWatchingOnly, errWatchingOnly, nil)
	}

	a.manager.mtx.Lock()
	defer a.manager.mtx.Unlock()

	// Account manager must be unlocked to decrypt the private key.
	if a.manager.rootManager.IsLocked() {
		return nil, managerError(ErrLocked, errLocked, nil)
	}

	// Decrypt the key as needed.  Also, make sure it's a copy since the
	// private key stored in memory can be cleared at any time.  Otherwise
	// the returned private key could be invalidated from under the caller.
	privKeyCopy, err := a.unlock(a.manager.rootManager.cryptoKeyPriv)
	if err != nil {
		return nil, err
	}

	privKey, _ := bchec.PrivKeyFromBytes(bchec.S256(), privKeyCopy)
	zero.Bytes(privKeyCopy)
	return privKey, nil
}

// ExportPrivKey returns the private key associated with the address in Wallet
// Import Format (WIF).
//
// This is part of the ManagedPubKeyAddress interface implementation.
func (a *managedAddress) ExportPrivKey() (*bchutil.WIF, error) {
	pk, err := a.PrivKey()
	if err != nil {
		return nil, err
	}

	return bchutil.NewWIF(pk, a.manager.rootManager.chainParams, a.compressed)
}

// Derivationinfo contains the information required to derive the key that
// backs the address via traditional methods from the HD root. For imported
// keys, the first value will be set to false to indicate that we don't know
// exactly how the key was derived.
//
// This is part of the ManagedPubKeyAddress interface implementation.
func (a *managedAddress) DerivationInfo() (KeyScope, DerivationPath, bool) {
	var (
		scope KeyScope
		path  DerivationPath
	)

	// If this key is imported, then we can't return any information as we
	// don't know precisely how the key was derived.
	if a.imported {
		return scope, path, false
	}

	return a.manager.Scope(), a.derivationPath, true
}

// newManagedAddressWithoutPrivKey returns a new managed address based on the
// passed account, public key, and whether or not the public key should be
// compressed.
func newManagedAddressWithoutPrivKey(m *ScopedKeyManager,
	derivationPath DerivationPath, pubKey *bchec.PublicKey, compressed bool,
	addrType AddressType) (*managedAddress, error) {

	// Create a pay-to-pubkey-hash address from the public key.
	var pubKeyHash []byte
	if compressed {
		pubKeyHash = bchutil.Hash160(pubKey.SerializeCompressed())
	} else {
		pubKeyHash = bchutil.Hash160(pubKey.SerializeUncompressed())
	}

	var address bchutil.Address
	var err error

	switch addrType {
	case PubKeyHash:
		address, err = bchutil.NewAddressPubKeyHash(
			pubKeyHash, m.rootManager.chainParams,
		)
		if err != nil {
			return nil, err
		}
	case RawPubKey:
		if compressed {
			address, err = bchutil.NewAddressPubKey(pubKey.SerializeCompressed(), m.rootManager.chainParams)
		} else {
			address, err = bchutil.NewAddressPubKey(pubKey.SerializeUncompressed(), m.rootManager.chainParams)
		}
	}

	return &managedAddress{
		manager:          m,
		address:          address,
		derivationPath:   derivationPath,
		imported:         false,
		internal:         false,
		addrType:         addrType,
		compressed:       compressed,
		pubKey:           pubKey,
		privKeyEncrypted: nil,
		privKeyCT:        nil,
	}, nil
}

// newManagedAddress returns a new managed address based on the passed account,
// private key, and whether or not the public key is compressed.  The managed
// address will have access to the private and public keys.
func newManagedAddress(s *ScopedKeyManager, derivationPath DerivationPath,
	privKey *bchec.PrivateKey, compressed bool,
	addrType AddressType) (*managedAddress, error) {

	// Encrypt the private key.
	//
	// NOTE: The privKeyBytes here are set into the managed address which
	// are cleared when locked, so they aren't cleared here.
	privKeyBytes := privKey.Serialize()
	privKeyEncrypted, err := s.rootManager.cryptoKeyPriv.Encrypt(privKeyBytes)
	if err != nil {
		str := "failed to encrypt private key"
		return nil, managerError(ErrCrypto, str, err)
	}

	// Leverage the code to create a managed address without a private key
	// and then add the private key to it.
	ecPubKey := (*bchec.PublicKey)(&privKey.PublicKey)
	managedAddr, err := newManagedAddressWithoutPrivKey(
		s, derivationPath, ecPubKey, compressed, addrType,
	)
	if err != nil {
		return nil, err
	}
	managedAddr.privKeyEncrypted = privKeyEncrypted
	managedAddr.privKeyCT = privKeyBytes

	return managedAddr, nil
}

// newManagedAddressFromExtKey returns a new managed address based on the passed
// account and extended key.  The managed address will have access to the
// private and public keys if the provided extended key is private, otherwise it
// will only have access to the public key.
func newManagedAddressFromExtKey(s *ScopedKeyManager,
	derivationPath DerivationPath, key *hdkeychain.ExtendedKey,
	addrType AddressType) (*managedAddress, error) {

	// Create a new managed address based on the public or private key
	// depending on whether the generated key is private.
	var managedAddr *managedAddress
	if key.IsPrivate() {
		privKey, err := key.ECPrivKey()
		if err != nil {
			return nil, err
		}

		// Ensure the temp private key big integer is cleared after
		// use.
		managedAddr, err = newManagedAddress(
			s, derivationPath, privKey, true, addrType,
		)
		if err != nil {
			return nil, err
		}
	} else {
		pubKey, err := key.ECPubKey()
		if err != nil {
			return nil, err
		}

		managedAddr, err = newManagedAddressWithoutPrivKey(
			s, derivationPath, pubKey, true,
			addrType,
		)
		if err != nil {
			return nil, err
		}
	}

	return managedAddr, nil
}

// scriptAddress represents a pay-to-script-hash address.
type scriptAddress struct {
	manager         *ScopedKeyManager
	account         uint32
	address         *bchutil.AddressScriptHash
	scriptEncrypted []byte
	scriptCT        []byte
	scriptMutex     sync.Mutex
	used            bool
}

// Enforce scriptAddress satisfies the ManagedScriptAddress interface.
var _ ManagedScriptAddress = (*scriptAddress)(nil)

// unlock decrypts and stores the associated script.  It will fail if the key is
// invalid or the encrypted script is not available.  The returned clear text
// script will always be a copy that may be safely used by the caller without
// worrying about it being zeroed during an address lock.
func (a *scriptAddress) unlock(key EncryptorDecryptor) ([]byte, error) {
	// Protect concurrent access to clear text script.
	a.scriptMutex.Lock()
	defer a.scriptMutex.Unlock()

	if len(a.scriptCT) == 0 {
		script, err := key.Decrypt(a.scriptEncrypted)
		if err != nil {
			str := fmt.Sprintf("failed to decrypt script for %s",
				a.address)
			return nil, managerError(ErrCrypto, str, err)
		}

		a.scriptCT = script
	}

	scriptCopy := make([]byte, len(a.scriptCT))
	copy(scriptCopy, a.scriptCT)
	return scriptCopy, nil
}

// lock zeroes the associated clear text private key.
func (a *scriptAddress) lock() {
	// Zero and nil the clear text script associated with this address.
	a.scriptMutex.Lock()
	zero.Bytes(a.scriptCT)
	a.scriptCT = nil
	a.scriptMutex.Unlock()
}

// Account returns the account the address is associated with.  This will always
// be the ImportedAddrAccount constant for script addresses.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Account() uint32 {
	return a.account
}

// AddrType returns the address type of the managed address. This can be used
// to quickly discern the address type without further processing
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) AddrType() AddressType {
	return Script
}

// Address returns the bchutil.Address which represents the managed address.
// This will be a pay-to-script-hash address.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Address() bchutil.Address {
	return a.address
}

// AddrHash returns the script hash for the address.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) AddrHash() []byte {
	return a.address.Hash160()[:]
}

// Imported always returns true since script addresses are always imported
// addresses and not part of any chain.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Imported() bool {
	return true
}

// Internal always returns false since script addresses are always imported
// addresses and not part of any chain in order to be for internal use.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Internal() bool {
	return false
}

// Compressed returns false since script addresses are never compressed.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Compressed() bool {
	return false
}

// Used returns true if the address has been used in a transaction.
//
// This is part of the ManagedAddress interface implementation.
func (a *scriptAddress) Used(ns walletdb.ReadBucket) bool {
	return a.manager.fetchUsed(ns, a.AddrHash())
}

// Script returns the script associated with the address.
//
// This implements the ScriptAddress interface.
func (a *scriptAddress) Script() ([]byte, error) {
	// No script is available for a watching-only address manager.
	if a.manager.rootManager.WatchOnly() {
		return nil, managerError(ErrWatchingOnly, errWatchingOnly, nil)
	}

	a.manager.mtx.Lock()
	defer a.manager.mtx.Unlock()

	// Account manager must be unlocked to decrypt the script.
	if a.manager.rootManager.IsLocked() {
		return nil, managerError(ErrLocked, errLocked, nil)
	}

	// Decrypt the script as needed.  Also, make sure it's a copy since the
	// script stored in memory can be cleared at any time.  Otherwise,
	// the returned script could be invalidated from under the caller.
	return a.unlock(a.manager.rootManager.cryptoKeyScript)
}

// newScriptAddress initializes and returns a new pay-to-script-hash address.
func newScriptAddress(m *ScopedKeyManager, account uint32, scriptHash,
	scriptEncrypted []byte) (*scriptAddress, error) {

	address, err := bchutil.NewAddressScriptHashFromHash(
		scriptHash, m.rootManager.chainParams,
	)
	if err != nil {
		return nil, err
	}

	return &scriptAddress{
		manager:         m,
		account:         account,
		address:         address,
		scriptEncrypted: scriptEncrypted,
	}, nil
}
