/** An error from Canon. */
declare class CanonError extends Error {
  /** Stable short name, for example "overlong-varint", "keys-out-of-order", or "text" for text that does not parse. */
  readonly code: string;
  /** Byte offset where the problem was found, or -1. */
  readonly offset: number;
}

type Bytes = Uint8Array | ArrayBuffer | ArrayBufferView;

interface Canon {
  /** Version of the Canon core, the same as the bxm-canon command. */
  readonly version: string;
  /** Canon text (JSON, plus h"hex" for bytes) to canonical bytes. */
  fromText(text: string): Uint8Array;
  /** Canonical bytes to Canon text. Throws CanonError if the bytes are not canonical. */
  toText(bytes: Bytes, opts?: { pretty?: boolean }): string;
  /** Text to its one normal form: keys sorted, no spaces (indented with pretty). */
  fmt(text: string, opts?: { pretty?: boolean }): string;
  /** Throws CanonError unless the bytes are exactly one canonical value. */
  check(bytes: Bytes): void;
  /** True when the bytes are exactly one canonical value. */
  isCanonical(bytes: Bytes): boolean;
  /** Lowercase hex SHA-256 of canonical bytes (checked first). */
  hash(bytes: Bytes): string;
  /** Lowercase hex SHA-256 of the canonical encoding of a text value. */
  hashText(text: string): string;
  /** Differences between two text values, one change per line, as printed by bxm-canon diff. */
  diffText(a: string, b: string): string[];
}

/** Loads the WebAssembly module (once) and returns the Canon API. */
declare function load(): Promise<Canon>;

export { CanonError, Canon, Bytes, load };
