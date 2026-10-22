/* Calls the C API the way another language's binding would. */
#include <stdio.h>
#include <string.h>
#include "bxm_canon.h"

static int fail(const char *what, char *err) {
    fprintf(stderr, "FAIL %s: %s\n", what, err ? err : "(no message)");
    bxm_canon_free_str(err);
    return 1;
}

int main(void) {
    const char *a = "{\"b\":[1,2.5,h\"ff\"],\"a\":\"x\"}";
    const char *b = "{ \"a\" : \"x\", \"b\" : [1, 2.5, h\"ff\"] }";
    uint8_t *ea = NULL, *eb = NULL, *t = NULL;
    size_t na = 0, nb = 0, nt = 0;
    char *err = NULL;
    char ha[65], hb[65];

    if (bxm_canon_from_text((const uint8_t *)a, strlen(a), &ea, &na, &err)) return fail("encode a", err);
    if (bxm_canon_from_text((const uint8_t *)b, strlen(b), &eb, &nb, &err)) return fail("encode b", err);
    if (na != nb || memcmp(ea, eb, na) != 0) return fail("same value, different bytes", NULL);
    if (bxm_canon_hash(ea, na, ha, &err) || bxm_canon_hash(eb, nb, hb, &err)) return fail("hash", err);
    if (strcmp(ha, hb) != 0) return fail("hashes differ", NULL);
    if (bxm_canon_to_text(ea, na, &t, &nt, &err)) return fail("to_text", err);
    if (nt != strlen("{\"a\":\"x\",\"b\":[1,2.5,h\"ff\"]}") ||
        memcmp(t, "{\"a\":\"x\",\"b\":[1,2.5,h\"ff\"]}", nt) != 0) return fail("text form", NULL);

    uint8_t overlong[] = {0x03, 0x80, 0x00};
    if (bxm_canon_check(overlong, sizeof overlong, &err) != BXM_ERR_DECODE) return fail("accepted overlong varint", NULL);
    if (!err || !strstr(err, "overlong-varint")) return fail("error message", err);
    bxm_canon_free_str(err);

    bxm_canon_free(ea, na);
    bxm_canon_free(eb, nb);
    bxm_canon_free(t, nt);
    printf("c api ok, version %s, hash %s\n", bxm_canon_version(), ha);
    return 0;
}
