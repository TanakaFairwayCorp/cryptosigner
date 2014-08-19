package main

import (
  "bytes"
  "errors"
  "crypto/aes"
  "crypto/cipher"
  "crypto/sha256"
  "encoding/hex"
)

// Internal representation of the address, public key and private key trifecta. The private key
// is still encrypted at this stage.
type key struct {
  address           string
  encryptedPrivate  []byte
  challenge         Challenge
}

func readKey(data []byte) *key {
  parts     := bytes.Split(data, []byte{32}) // space
  encpkey,_ := hex.DecodeString(string(parts[1]))
  challng,_ := hex.DecodeString(string(parts[2]))
  return &key{string(parts[0]), encpkey, ReadChallenge(challng)}
}

func (self *key) bytes() []byte {
  data := bytes.NewBuffer([]byte(self.address))
  data.WriteString(" ")
  data.WriteString(hex.EncodeToString(self.encryptedPrivate))
  data.WriteString(" ")
  data.WriteString(hex.EncodeToString(self.challenge.Bytes()))
  return data.Bytes()
}

// Holds the keys and handles their lifecycle. Decrypts the private key just for the time of
// computing a signature.
type Hold struct {
  cipher  cipher.Block
  store   Store
  signer  Signer
  keys    map[string]*key
}

func MakeHold(pass string, store Store, signer Signer) (*Hold, error) {
  // hash the password to make a 32-bytes cipher key
  passh   := sha256.Sum256([]byte(pass))
  cipher, err  := aes.NewCipher(passh[:])
  if err != nil { return nil, err }
  data, err := store.ReadAll()
  if err != nil { return nil, err }

  keys := readKeyData(data)
  return &Hold{cipher, store, signer, keys}, nil
}

func (self *Hold) NewKey(challenge Challenge, prefix byte) (string, error) {
  pub, priv, err := self.signer.NewKey()
  if err != nil { return "", err }
  addr := EncodeAddress(hash160(pub), prefix)

  enc, err := encrypt(self.cipher, priv)
  if err != nil { return "", err }
  newkey := &key{addr, enc, challenge}
  self.keys[addr] = newkey
  return addr, self.store.Save(string(addr), newkey.bytes())
}

func (self *Hold) Sign(addr string, data []byte) ([]byte, error) {
  key   := self.keys[addr]
  if key == nil {
    return nil, errors.New("Unknown address: " + addr)
  }
  if !key.challenge.Check(data) {
    return nil, errors.New("challenge failed")
  }

  priv, err  := decrypt(self.cipher, key.encryptedPrivate)
  if err != nil { return nil, err }

  // data passed is the digested tx bytes to sign, what we sign is the double-sha of that
  sigBytes := append(data, []byte{1, 0, 0, 0}...)
  return self.signer.Sign(priv, doubleHash(sigBytes))
}

func readKeyData(data [][]byte) map[string]*key {
  keys := make(map[string]*key)
  for _, kd := range data {
    key := readKey(kd)
    keys[key.address] = key
  }
  return keys
}
