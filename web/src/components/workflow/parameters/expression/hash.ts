/**
 * Dependency-free hashing helpers.
 *
 * m9m's server registers `md5`, `sha1`, `sha256` and `sha512`
 * (`internal/expressions/functions.go`). The browser preview has to
 * produce the same digests, but `crypto.subtle` is asynchronous and the
 * expression evaluator is synchronous, so the digests are implemented
 * here as plain TypeScript.
 *
 * The implementations follow FIPS 180-4 (SHA-1/SHA-2) and RFC 1321
 * (MD5). Constants that are usually hard-coded (the SHA-2 round
 * constants) are derived at load time from the primes they are defined
 * by, which keeps the file short and self-checking.
 */

const textEncoder = new TextEncoder()
const textDecoder = new TextDecoder()

/** UTF-8 bytes of a string. */
function utf8Bytes(input: string): Uint8Array {
  return textEncoder.encode(input)
}

/** Lower-case hex representation of a byte array. */
function toHex(bytes: Uint8Array): string {
  let out = ''
  for (const byte of bytes) {
    out += byte.toString(16).padStart(2, '0')
  }
  return out
}

/** Rotate a 32-bit word right. */
function rotr32(value: number, shift: number): number {
  return ((value >>> shift) | (value << (32 - shift))) >>> 0
}

/** Rotate a 32-bit word left. */
function rotl32(value: number, shift: number): number {
  return ((value << shift) | (value >>> (32 - shift))) >>> 0
}

/** The first `count` prime numbers. */
function firstPrimes(count: number): number[] {
  const primes: number[] = []
  for (let candidate = 2; primes.length < count; candidate += 1) {
    let isPrime = true
    for (let divisor = 2; divisor * divisor <= candidate; divisor += 1) {
      if (candidate % divisor === 0) {
        isPrime = false
        break
      }
    }
    if (isPrime) primes.push(candidate)
  }
  return primes
}

/** Integer cube root (`floor(cbrt(n))`) computed with BigInt Newton iteration. */
function integerCubeRoot(value: bigint): bigint {
  if (value < 2n) return value
  let guess = 1n << BigInt(Math.ceil(value.toString(2).length / 3) + 1)
  for (;;) {
    const next = (2n * guess + value / (guess * guess)) / 3n
    if (next >= guess) break
    guess = next
  }
  while (guess * guess * guess > value) guess -= 1n
  while ((guess + 1n) ** 3n <= value) guess += 1n
  return guess
}

/** Integer square root (`floor(sqrt(n))`) computed with BigInt Newton iteration. */
function integerSquareRoot(value: bigint): bigint {
  if (value < 2n) return value
  let guess = 1n << BigInt(Math.ceil(value.toString(2).length / 2) + 1)
  for (;;) {
    const next = (guess + value / guess) >> 1n
    if (next >= guess) break
    guess = next
  }
  while (guess * guess > value) guess -= 1n
  while ((guess + 1n) * (guess + 1n) <= value) guess += 1n
  return guess
}

/** SHA-256 round constants: first 32 bits of the fractional cube root of the i-th prime. */
const SHA256_K = (() => {
  const constants = new Uint32Array(64)
  const primes = firstPrimes(64)
  for (let index = 0; index < 64; index += 1) {
    const scaled = integerCubeRoot(BigInt(primes[index]) << 96n)
    constants[index] = Number(scaled & 0xffffffffn) >>> 0
  }
  return constants
})()

/** SHA-512 round constants: first 64 bits of the fractional cube root of the i-th prime. */
const SHA512_K = (() => {
  const constants: bigint[] = []
  const primes = firstPrimes(80)
  const mask = (1n << 64n) - 1n
  for (let index = 0; index < 80; index += 1) {
    const scaled = integerCubeRoot(BigInt(primes[index]) << 192n)
    constants.push(scaled & mask)
  }
  return constants
})()

/** SHA-512 initial hash values: first 64 bits of the fractional square root of the i-th prime. */
const SHA512_H = (() => {
  const values: bigint[] = []
  const primes = firstPrimes(8)
  const mask = (1n << 64n) - 1n
  for (const prime of primes) {
    values.push(integerSquareRoot(BigInt(prime) << 128n) & mask)
  }
  return values
})()

/** MD5 per-round shift amounts. */
const MD5_SHIFTS = [
  7, 12, 17, 22, 5, 9, 14, 20, 4, 11, 16, 23, 6, 10, 15, 21,
]

/** MD5 round constants: `floor(abs(sin(i + 1)) * 2^32)`. */
const MD5_K = (() => {
  const constants = new Uint32Array(64)
  for (let index = 0; index < 64; index += 1) {
    constants[index] = Math.floor(Math.abs(Math.sin(index + 1)) * 4294967296) >>> 0
  }
  return constants
})()

/** Append the MD5/SHA padding (0x80, zeros, 64-bit length) to a message. */
function padMessage(bytes: Uint8Array, lengthBytes: 8 | 16): Uint8Array {
  const blockSize = 64
  const paddedLength = Math.ceil((bytes.length + 1 + lengthBytes) / blockSize) * blockSize
  const padded = new Uint8Array(paddedLength)
  padded.set(bytes)
  padded[bytes.length] = 0x80
  const view = new DataView(padded.buffer)
  const bitLength = BigInt(bytes.length) * 8n
  if (lengthBytes === 8) {
    view.setUint32(paddedLength - 8, Number(bitLength & 0xffffffffn), true)
    view.setUint32(paddedLength - 4, Number((bitLength >> 32n) & 0xffffffffn), true)
  } else {
    view.setBigUint64(paddedLength - 8, bitLength, true)
  }
  return padded
}

/** MD5 digest of a string (RFC 1321). */
export function md5(input: string): string {
  const padded = padMessage(utf8Bytes(input), 8)
  const view = new DataView(padded.buffer)
  const words = new Uint32Array(16)

  let a0 = 0x67452301
  let b0 = 0xefcdab89
  let c0 = 0x98badcfe
  let d0 = 0x10325476

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getUint32(offset + index * 4, true)
    }

    let a = a0
    let b = b0
    let c = c0
    let d = d0

    for (let index = 0; index < 64; index += 1) {
      let f: number
      let g: number
      if (index < 16) {
        f = (b & c) | (~b & d)
        g = index
      } else if (index < 32) {
        f = (d & b) | (~d & c)
        g = (5 * index + 1) % 16
      } else if (index < 48) {
        f = b ^ c ^ d
        g = (3 * index + 5) % 16
      } else {
        f = c ^ (b | ~d)
        g = (7 * index) % 16
      }
      f = (f + a + MD5_K[index] + words[g]) >>> 0
      a = d
      d = c
      c = b
      b = (b + rotl32(f, MD5_SHIFTS[index])) >>> 0
    }

    a0 = (a0 + a) >>> 0
    b0 = (b0 + b) >>> 0
    c0 = (c0 + c) >>> 0
    d0 = (d0 + d) >>> 0
  }

  const digest = new Uint8Array(16)
  const digestView = new DataView(digest.buffer)
  digestView.setUint32(0, a0, true)
  digestView.setUint32(4, b0, true)
  digestView.setUint32(8, c0, true)
  digestView.setUint32(12, d0, true)
  return toHex(digest)
}

/** SHA-1 digest of a string (FIPS 180-4). */
export function sha1(input: string): string {
  const padded = padMessage(utf8Bytes(input), 8)
  const view = new DataView(padded.buffer)
  const words = new Uint32Array(80)

  let h0 = 0x67452301
  let h1 = 0xefcdab89
  let h2 = 0x98badcfe
  let h3 = 0x10325476
  let h4 = 0xc3d2e1f0

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getUint32(offset + index * 4, true)
    }
    for (let index = 16; index < 80; index += 1) {
      words[index] = rotl32(
        words[index - 3] ^ words[index - 8] ^ words[index - 14] ^ words[index - 16],
        1,
      )
    }

    let a = h0
    let b = h1
    let c = h2
    let d = h3
    let e = h4

    for (let index = 0; index < 80; index += 1) {
      let f: number
      let k: number
      if (index < 20) {
        f = (b & c) | (~b & d)
        k = 0x5a827999
      } else if (index < 40) {
        f = b ^ c ^ d
        k = 0x6ed9eba1
      } else if (index < 60) {
        f = (b & c) | (b & d) | (c & d)
        k = 0x8f1bbcdc
      } else {
        f = b ^ c ^ d
        k = 0xca62c1d6
      }
      const temp = (rotl32(a, 5) + f + e + k + words[index]) >>> 0
      e = d
      d = c
      c = rotl32(b, 30)
      b = a
      a = temp
    }

    h0 = (h0 + a) >>> 0
    h1 = (h1 + b) >>> 0
    h2 = (h2 + c) >>> 0
    h3 = (h3 + d) >>> 0
    h4 = (h4 + e) >>> 0
  }

  const digest = new Uint8Array(20)
  const digestView = new DataView(digest.buffer)
  digestView.setUint32(0, h0, true)
  digestView.setUint32(4, h1, true)
  digestView.setUint32(8, h2, true)
  digestView.setUint32(12, h3, true)
  digestView.setUint32(16, h4, true)
  return toHex(digest)
}

/** SHA-256 digest of a string (FIPS 180-4). */
export function sha256(input: string): string {
  const padded = padMessage(utf8Bytes(input), 8)
  const view = new DataView(padded.buffer)
  const words = new Uint32Array(64)

  const hash = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getUint32(offset + index * 4, true)
    }
    for (let index = 16; index < 64; index += 1) {
      const w15 = words[index - 15]
      const w2 = words[index - 2]
      const s0 = rotr32(w15, 7) ^ rotr32(w15, 18) ^ (w15 >>> 3)
      const s1 = rotr32(w2, 17) ^ rotr32(w2, 19) ^ (w2 >>> 10)
      words[index] = (words[index - 16] + s0 + words[index - 7] + s1) >>> 0
    }

    let a = hash[0]
    let b = hash[1]
    let c = hash[2]
    let d = hash[3]
    let e = hash[4]
    let f = hash[5]
    let g = hash[6]
    let h = hash[7]

    for (let index = 0; index < 64; index += 1) {
      const s1 = rotr32(e, 6) ^ rotr32(e, 11) ^ rotr32(e, 25)
      const ch = (e & f) ^ (~e & g)
      const temp1 = (h + s1 + ch + SHA256_K[index] + words[index]) >>> 0
      const s0 = rotr32(a, 2) ^ rotr32(a, 13) ^ rotr32(a, 22)
      const maj = (a & b) ^ (a & c) ^ (b & c)
      const temp2 = (s0 + maj) >>> 0

      h = g
      g = f
      f = e
      e = (d + temp1) >>> 0
      d = c
      c = b
      b = a
      a = (temp1 + temp2) >>> 0
    }

    hash[0] = (hash[0] + a) >>> 0
    hash[1] = (hash[1] + b) >>> 0
    hash[2] = (hash[2] + c) >>> 0
    hash[3] = (hash[3] + d) >>> 0
    hash[4] = (hash[4] + e) >>> 0
    hash[5] = (hash[5] + f) >>> 0
    hash[6] = (hash[6] + g) >>> 0
    hash[7] = (hash[7] + h) >>> 0
  }

  const digest = new Uint8Array(32)
  const digestView = new DataView(digest.buffer)
  for (let index = 0; index < 8; index += 1) {
    digestView.setUint32(index * 4, hash[index], true)
  }
  return toHex(digest)
}

/** Rotate a 64-bit BigInt right. */
function rotr64(value: bigint, shift: bigint): bigint {
  const mask = (1n << 64n) - 1n
  return ((value >> shift) | (value << (64n - shift))) & mask
}

/** SHA-512 digest of a string (FIPS 180-4). */
export function sha512(input: string): string {
  const padded = padMessage(utf8Bytes(input), 16)
  const view = new DataView(padded.buffer)
  const words: bigint[] = new Array(80).fill(0n)
  const mask = (1n << 64n) - 1n

  const hash = SHA512_H.slice()

  for (let offset = 0; offset < padded.length; offset += 128) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getBigUint64(offset + index * 8, true)
    }
    for (let index = 16; index < 80; index += 1) {
      const w15 = words[index - 15]
      const w2 = words[index - 2]
      const s0 = rotr64(w15, 1n) ^ rotr64(w15, 8n) ^ (w15 >> 7n)
      const s1 = rotr64(w2, 19n) ^ rotr64(w2, 61n) ^ (w2 >> 6n)
      words[index] = (words[index - 16] + s0 + words[index - 7] + s1) & mask
    }

    let a = hash[0]
    let b = hash[1]
    let c = hash[2]
    let d = hash[3]
    let e = hash[4]
    let f = hash[5]
    let g = hash[6]
    let h = hash[7]

    for (let index = 0; index < 80; index += 1) {
      const s1 = rotr64(e, 14n) ^ rotr64(e, 18n) ^ rotr64(e, 41n)
      const ch = (e & f) ^ (~e & mask & g)
      const temp1 = (h + s1 + ch + SHA512_K[index] + words[index]) & mask
      const s0 = rotr64(a, 28n) ^ rotr64(a, 34n) ^ rotr64(a, 39n)
      const maj = (a & b) ^ (a & c) ^ (b & c)
      const temp2 = (s0 + maj) & mask

      h = g
      g = f
      f = e
      e = (d + temp1) & mask
      d = c
      c = b
      b = a
      a = (temp1 + temp2) & mask
    }

    hash[0] = (hash[0] + a) & mask
    hash[1] = (hash[1] + b) & mask
    hash[2] = (hash[2] + c) & mask
    hash[3] = (hash[3] + d) & mask
    hash[4] = (hash[4] + e) & mask
    hash[5] = (hash[5] + f) & mask
    hash[6] = (hash[6] + g) & mask
    hash[7] = (hash[7] + h) & mask
  }

  let out = ''
  for (const word of hash) {
    out += word.toString(16).padStart(16, '0')
  }
  return out
}

const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

/** Base64-encode a UTF-8 string (mirrors Go's `base64.StdEncoding`). */
export function base64Encode(input: string): string {
  const bytes = utf8Bytes(input)
  let out = ''
  for (let index = 0; index < bytes.length; index += 3) {
    const byte0 = bytes[index]
    const byte1 = bytes[index + 1]
    const byte2 = bytes[index + 2]
    out += BASE64_ALPHABET[byte0 >> 2]
    out += BASE64_ALPHABET[((byte0 & 0x03) << 4) | ((byte1 ?? 0) >> 4)]
    out += byte1 === undefined ? '=' : BASE64_ALPHABET[((byte1 & 0x0f) << 2) | ((byte2 ?? 0) >> 6)]
    out += byte2 === undefined ? '=' : BASE64_ALPHABET[byte2 & 0x3f]
  }
  return out
}

/** Decode a Base64 string into UTF-8 text. Throws on invalid input. */
export function base64Decode(input: string): string {
  const cleaned = input.replace(/[^A-Za-z0-9+/]/g, '')
  const bytes: number[] = []
  for (let index = 0; index < cleaned.length; index += 4) {
    const char0 = BASE64_ALPHABET.indexOf(cleaned[index])
    const char1 = BASE64_ALPHABET.indexOf(cleaned[index + 1] ?? 'A')
    const char2 = BASE64_ALPHABET.indexOf(cleaned[index + 2] ?? 'A')
    const char3 = BASE64_ALPHABET.indexOf(cleaned[index + 3] ?? 'A')
    if (char0 < 0 || char1 < 0) throw new Error('DecodeError: Invalid base64 string')
    bytes.push(((char0 << 2) | (char1 >> 4)) & 0xff)
    if (cleaned[index + 2] !== undefined) bytes.push(((char1 & 0x0f) << 4) | (char2 >> 2))
    if (cleaned[index + 3] !== undefined) bytes.push(((char2 & 0x03) << 6) | char3)
  }
  return textDecoder.decode(new Uint8Array(bytes))
}
