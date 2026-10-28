//go:build darwin && cgo

// Package ownerkey provides explicit native custody for the owner signing key.
// Device keys and ordinary archive access are deliberately separate.
package ownerkey

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef cm_query(const char *account) {
 CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
   &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
 CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
 CFDictionarySetValue(q, kSecAttrService, CFSTR("io.capsule-memory.owner.v1"));
 CFDictionarySetValue(q, kSecAttrAccount, a);
 CFDictionarySetValue(q, kSecAttrSynchronizable, kCFBooleanFalse);
 CFRelease(a);
 return q;
}
static int cm_put(const char *account, const void *secret, int n) {
 CFMutableDictionaryRef q = cm_query(account);
 CFDataRef data = CFDataCreate(NULL, secret, n);
 CFDictionarySetValue(q, kSecValueData, data);
 OSStatus status = SecItemAdd(q, NULL);
 CFRelease(data); CFRelease(q);
 return (int)status;
}
static int cm_get(const char *account, unsigned char **out, long *n) {
 CFMutableDictionaryRef q = cm_query(account);
 CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
 CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
 CFTypeRef result = NULL;
 OSStatus status = SecItemCopyMatching(q, &result);
 CFRelease(q);
 if (status != errSecSuccess) return (int)status;
 if (!result || CFGetTypeID(result) != CFDataGetTypeID()) {
   if (result) CFRelease(result);
   return -1;
 }
 *n = CFDataGetLength((CFDataRef)result);
 if (*n != 64) { CFRelease(result); return -1; }
 *out = malloc((size_t)*n);
 if (!*out) { CFRelease(result); return -1; }
 memcpy(*out, CFDataGetBytePtr((CFDataRef)result), (size_t)*n);
 CFRelease(result);
 return 0;
}
static void cm_free(unsigned char *p, long n) { memset(p, 0, (size_t)n); free(p); }
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"
)

func Get(account string) ([]byte, error) {
	a := C.CString(account)
	defer C.free(unsafe.Pointer(a))
	var data *C.uchar
	var n C.long
	status := C.cm_get(a, &data, &n)
	if status != 0 {
		return nil, fmt.Errorf("owner Keychain read failed (OSStatus %d); no file fallback", int(status))
	}
	defer C.cm_free(data, n)
	return C.GoBytes(unsafe.Pointer(data), C.int(n)), nil
}

func Put(account string, secret []byte) error {
	if len(secret) != 64 {
		return errors.New("invalid owner key length")
	}
	a := C.CString(account)
	defer C.free(unsafe.Pointer(a))
	status := C.cm_put(a, unsafe.Pointer(&secret[0]), C.int(len(secret)))
	if status == -25299 {
		existing, err := Get(account)
		if err != nil {
			return err
		}
		if bytes.Equal(existing, secret) {
			return nil
		}
		return errors.New("owner Keychain entry already contains a different key")
	}
	if status != 0 {
		return fmt.Errorf("owner Keychain write failed (OSStatus %d)", int(status))
	}
	return nil
}
