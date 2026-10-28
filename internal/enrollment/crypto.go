package enrollment

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
)

type Identity struct {
	Signing    string `json:"signing_private_key_b64" cm:"required"`
	Encryption string `json:"encryption_private_key_b64" cm:"required"`
}

func NewIdentity(i Invitation) (Identity, Signed, error) {
	if i.signedDigest == "" {
		return Identity{}, Signed{}, ErrInvalid
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, Signed{}, err
	}
	enc, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, Signed{}, err
	}
	device, err := RandomID()
	if err != nil {
		return Identity{}, Signed{}, err
	}
	r := Request{Version: 1, Vault: i.Vault, Invite: i.ID, InvitationDigest: i.signedDigest, Device: device, Public: base64.StdEncoding.EncodeToString(pub), Encryption: base64.StdEncoding.EncodeToString(enc.PublicKey().Bytes())}
	s, err := Sign(RequestDomain, r, priv)
	return Identity{base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(enc.Bytes())}, s, err
}

func (k Identity) Validate(i Invitation, signed Signed) (ed25519.PrivateKey, error) {
	r, err := ParseRequest(signed, i)
	if err != nil {
		return nil, err
	}
	priv, err := DecodeKey(k.Signing, ed25519.PrivateKeySize)
	if err != nil {
		return nil, err
	}
	generated := ed25519.NewKeyFromSeed(priv[:32])
	if !ed25519.PrivateKey(priv).Equal(generated) || base64.StdEncoding.EncodeToString(generated.Public().(ed25519.PublicKey)) != r.Public {
		return nil, ErrInvalid
	}
	enc, err := DecodeKey(k.Encryption, 32)
	if err != nil {
		return nil, err
	}
	x, err := ecdh.X25519().NewPrivateKey(enc)
	if err != nil || base64.StdEncoding.EncodeToString(x.PublicKey().Bytes()) != r.Encryption {
		return nil, ErrInvalid
	}
	return ed25519.PrivateKey(priv), nil
}

func keyInfo(i Invitation, r Signed) []byte {
	return []byte("capsule-memory/enrollment-key/v1\n" + i.Vault + "\n" + i.ID + "\n" + Digest(r) + "\n")
}

// Seal uses the RFC 9180 base-mode DHKEM(X25519,HKDF-SHA256), HKDF-SHA256,
// AES-256-GCM suite. The separately owner-signed decision authenticates sender
// identity, scope selection, control, ciphertext and the entire request context.
func Seal(i Invitation, r Signed, plain []byte) (string, error) {
	if len(plain) > MaxPlaintext {
		return "", ErrInvalid
	}
	request, err := ParseRequest(r, i)
	if err != nil {
		return "", err
	}
	key, err := DecodeKey(request.Encryption, 32)
	if err != nil {
		return "", err
	}
	x, err := ecdh.X25519().NewPublicKey(key)
	if err != nil {
		return "", ErrInvalid
	}
	pub, err := hpke.NewDHKEMPublicKey(x)
	if err != nil {
		return "", ErrInvalid
	}
	ct, err := hpke.Seal(pub, hpke.HKDFSHA256(), hpke.AES256GCM(), keyInfo(i, r), plain)
	if err != nil {
		return "", ErrInvalid
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func Open(i Invitation, r Signed, k Identity, ciphertext string) ([]byte, error) {
	if _, err := k.Validate(i, r); err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil || base64.StdEncoding.EncodeToString(ct) != ciphertext || len(ct) > MaxPlaintext+128 {
		return nil, ErrInvalid
	}
	key, err := DecodeKey(k.Encryption, 32)
	if err != nil {
		return nil, err
	}
	x, err := ecdh.X25519().NewPrivateKey(key)
	if err != nil {
		return nil, ErrInvalid
	}
	priv, err := hpke.NewDHKEMPrivateKey(x)
	if err != nil {
		return nil, ErrInvalid
	}
	plain, err := hpke.Open(priv, hpke.HKDFSHA256(), hpke.AES256GCM(), keyInfo(i, r), ct)
	if err != nil || len(plain) > MaxPlaintext {
		return nil, ErrInvalid
	}
	return plain, nil
}
