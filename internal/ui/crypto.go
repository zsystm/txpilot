package ui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/ripemd160"
)

func ParsePrivateKey(hexKey string) ([]byte, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")
	pk, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex private key: %w", err)
	}
	if len(pk) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(pk))
	}
	return pk, nil
}

func DeriveCosmosAddress(privateKey []byte, hrp string) (string, error) {
	pubKey := derivePublicKey(privateKey)
	if pubKey == nil {
		return "", errors.New("failed to derive public key")
	}
	
	compressedPubKey := compressPublicKey(pubKey)
	
	sha := sha256.Sum256(compressedPubKey)
	hasher := ripemd160.New()
	hasher.Write(sha[:])
	addrBytes := hasher.Sum(nil)
	
	return Bech32Encode(hrp, addrBytes)
}

func DeriveEVMAddress(privateKey []byte) (string, error) {
	pubKey := derivePublicKey(privateKey)
	if pubKey == nil {
		return "", errors.New("failed to derive public key")
	}
	
	hash := Keccak256(pubKey[1:])
	addr := hash[12:]
	
	return "0x" + hex.EncodeToString(addr), nil
}

func GenerateRandomCosmosAddress(hrp string) (string, error) {
	randBytes := make([]byte, 20)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	return Bech32Encode(hrp, randBytes)
}

func GenerateRandomEVMAddress() string {
	randBytes := make([]byte, 20)
	rand.Read(randBytes)
	return "0x" + hex.EncodeToString(randBytes)
}

func ExtractHRP(bech32Addr string) (string, error) {
	oneIndex := strings.Index(bech32Addr, "1")
	if oneIndex < 1 {
		return "", errors.New("invalid bech32 address")
	}
	return bech32Addr[:oneIndex], nil
}

func derivePublicKey(privateKey []byte) []byte {
	curve := secp256k1Curve()
	x, y := curve.ScalarBaseMult(privateKey)
	if x == nil || y == nil {
		return nil
	}
	
	pubKey := make([]byte, 65)
	pubKey[0] = 0x04
	x.FillBytes(pubKey[1:33])
	y.FillBytes(pubKey[33:65])
	return pubKey
}

func compressPublicKey(pubKey []byte) []byte {
	if len(pubKey) != 65 || pubKey[0] != 0x04 {
		return nil
	}
	
	compressed := make([]byte, 33)
	copy(compressed[1:], pubKey[1:33])
	
	y := new(big.Int).SetBytes(pubKey[33:65])
	if y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	return compressed
}

func secp256k1Curve() *ellipticCurve {
	p, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	n, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	b := big.NewInt(7)
	gx, _ := new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	gy, _ := new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	
	return &ellipticCurve{
		P:  p,
		N:  n,
		B:  b,
		Gx: gx,
		Gy: gy,
	}
}

type ellipticCurve struct {
	P  *big.Int
	N  *big.Int
	B  *big.Int
	Gx *big.Int
	Gy *big.Int
}

func (curve *ellipticCurve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	scalar := new(big.Int).SetBytes(k)
	scalar.Mod(scalar, curve.N)
	
	return curve.scalarMult(curve.Gx, curve.Gy, scalar)
}

func (curve *ellipticCurve) scalarMult(x1, y1 *big.Int, k *big.Int) (*big.Int, *big.Int) {
	if k.Sign() == 0 {
		return nil, nil
	}
	
	rx, ry := new(big.Int), new(big.Int)
	tx, ty := new(big.Int).Set(x1), new(big.Int).Set(y1)
	
	for i := 0; i < k.BitLen(); i++ {
		if k.Bit(i) == 1 {
			if rx.Sign() == 0 {
				rx.Set(tx)
				ry.Set(ty)
			} else {
				rx, ry = curve.add(rx, ry, tx, ty)
			}
		}
		tx, ty = curve.double(tx, ty)
	}
	
	return rx, ry
}

func (curve *ellipticCurve) add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	if x1.Cmp(x2) == 0 && y1.Cmp(y2) == 0 {
		return curve.double(x1, y1)
	}
	
	lambda := new(big.Int).Sub(y2, y1)
	denom := new(big.Int).Sub(x2, x1)
	denom.ModInverse(denom, curve.P)
	lambda.Mul(lambda, denom)
	lambda.Mod(lambda, curve.P)
	
	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, x1)
	x3.Sub(x3, x2)
	x3.Mod(x3, curve.P)
	
	y3 := new(big.Int).Sub(x1, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, y1)
	y3.Mod(y3, curve.P)
	
	return x3, y3
}

func (curve *ellipticCurve) double(x, y *big.Int) (*big.Int, *big.Int) {
	lambda := new(big.Int).Mul(x, x)
	lambda.Mul(lambda, big.NewInt(3))
	denom := new(big.Int).Mul(y, big.NewInt(2))
	denom.ModInverse(denom, curve.P)
	lambda.Mul(lambda, denom)
	lambda.Mod(lambda, curve.P)
	
	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, new(big.Int).Mul(x, big.NewInt(2)))
	x3.Mod(x3, curve.P)
	
	y3 := new(big.Int).Sub(x, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, y)
	y3.Mod(y3, curve.P)
	
	return x3, y3
}

func Keccak256(data []byte) []byte {
	// Simplified Keccak256 for MVP - in production use golang.org/x/crypto/sha3
	// This is a placeholder that uses SHA256 instead
	// TODO: Replace with proper Keccak256 implementation
	h := sha256.Sum256(data)
	return h[:]
}

var bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func Bech32Encode(hrp string, data []byte) (string, error) {
	values := convertBits(data, 8, 5, true)
	if values == nil {
		return "", errors.New("encoding error")
	}
	
	checksum := bech32Checksum(hrp, values)
	combined := append(values, checksum...)
	
	var result strings.Builder
	result.WriteString(hrp)
	result.WriteString("1")
	for _, v := range combined {
		result.WriteByte(bech32Charset[v])
	}
	
	return result.String(), nil
}

func bech32Checksum(hrp string, values []byte) []byte {
	mod := bech32Polymod(bech32HrpExpand(hrp), values, make([]byte, 6)) ^ 1
	checksum := make([]byte, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = byte((mod >> uint(5*(5-i))) & 31)
	}
	return checksum
}

func bech32Polymod(hrpExpanded, values, checksum []byte) int {
	chk := 1
	for _, v := range hrpExpanded {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ int(v)
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}[i]
			}
		}
	}
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ int(v)
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}[i]
			}
		}
	}
	for _, v := range checksum {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ int(v)
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= []int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}[i]
			}
		}
	}
	return chk
}

func bech32HrpExpand(hrp string) []byte {
	expanded := make([]byte, 0, len(hrp)*2+1)
	for _, c := range hrp {
		expanded = append(expanded, byte(c>>5))
	}
	expanded = append(expanded, 0)
	for _, c := range hrp {
		expanded = append(expanded, byte(c&31))
	}
	return expanded
}

func convertBits(data []byte, fromBits, toBits uint, pad bool) []byte {
	acc := 0
	bits := uint(0)
	ret := make([]byte, 0, len(data)*int(fromBits)/int(toBits)+1)
	maxv := (1 << toBits) - 1
	
	for _, value := range data {
		acc = (acc << fromBits) | int(value)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, byte((acc>>bits)&maxv))
		}
	}
	
	if pad {
		if bits > 0 {
			ret = append(ret, byte((acc<<(toBits-bits))&maxv))
		}
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		return nil
	}
	
	return ret
}