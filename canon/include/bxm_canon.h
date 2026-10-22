/* BXM Canon C API. Licensed under the Apache License 2.0.
 *
 * Link against libbxm_canon (static or shared). Every function returns 0 on
 * success. Buffers handed out must be released with bxm_canon_free, error
 * messages with bxm_canon_free_str. Pass NULL for err_out to skip messages.
 */
#ifndef BXM_CANON_H
#define BXM_CANON_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define BXM_OK 0
#define BXM_ERR_TEXT 1
#define BXM_ERR_ENCODE 2
#define BXM_ERR_DECODE 3
#define BXM_ERR_ARG 4

/* Canon text (JSON plus h"hex") in, canonical bytes out. */
int bxm_canon_from_text(const uint8_t *text, size_t text_len,
                        uint8_t **out, size_t *out_len, char **err_out);

/* Canonical bytes in, compact Canon text out (not NUL-terminated). */
int bxm_canon_to_text(const uint8_t *data, size_t data_len,
                      uint8_t **out, size_t *out_len, char **err_out);

/* 0 when data is exactly one canonical value. */
int bxm_canon_check(const uint8_t *data, size_t data_len, char **err_out);

/* Checks data, then writes 64 hex chars of SHA-256 plus a NUL to hex_out[65]. */
int bxm_canon_hash(const uint8_t *data, size_t data_len, char *hex_out,
                   char **err_out);

void bxm_canon_free(uint8_t *p, size_t len);
void bxm_canon_free_str(char *p);
const char *bxm_canon_version(void);

#ifdef __cplusplus
}
#endif

#endif
