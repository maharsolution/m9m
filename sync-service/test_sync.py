#!/usr/bin/env python3
"""Unit tests for sync.py — focuses on the AES-256-CBC decrypt
function. Cross-checks with the Go implementation in
internal/credentials/n8n_cipher_test.go against the same known
ciphertext vector.

Run: `python sync-service/test_sync.py` from the repo root.
"""
import os
import sys

# Make `import sync` work when invoked from repo root.
sys.path.insert(0, os.path.join(os.path.dirname(__file__)))

import sync


def test_decrypt_known_vector():
    """Known ciphertext from a live n8n Postgres DB on 2026-09-09
    (the `basic_credential` envelope). With the right
    N8N_ENCRYPTION_KEY we expect `{"user":"admin","password":"admin123"}`.
    """
    os.environ["N8N_ENCRYPTION_KEY"] = "Default_Secure_Key_Change_Me_!!!"
    # Re-read env into the module-level constant. Python imports
    # happen at module load time, so the env must be set before
    # import — we do this here by re-reading the constant manually.
    sync.N8N_ENCRYPTION_KEY = os.environ["N8N_ENCRYPTION_KEY"]
    ciphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
    pt = sync.decrypt_n8n_credential_data(ciphertext)
    assert pt == {"user": "admin", "password": "admin123"}, f"got {pt!r}"


def test_decrypt_empty():
    """Empty ciphertext -> empty dict. n8n returns "" for
    credentials that have no data (e.g. a newly-created form)."""
    sync.N8N_ENCRYPTION_KEY = "Default_Secure_Key_Change_Me_!!!"
    assert sync.decrypt_n8n_credential_data("") == {}


def test_decrypt_no_key_raises():
    """Missing N8N_ENCRYPTION_KEY must raise rather than silently
    returning junk — silent failure would have us push opaque
    ciphertext blobs to m9m and break the webhook auth path."""
    sync.N8N_ENCRYPTION_KEY = ""
    try:
        sync.decrypt_n8n_credential_data("anything")
    except RuntimeError as e:
        assert "N8N_ENCRYPTION_KEY" in str(e), f"unexpected error: {e}"
    else:
        raise AssertionError("expected RuntimeError when N8N_ENCRYPTION_KEY is unset")


def test_decrypt_wrong_key_returns_garbage():
    """With a wrong key, AES-CBC produces 32 bytes of garbage
    that PKCS#7 unpad will reject. We verify the function raises
    rather than returning the wrong plaintext — silently returning
    garbage would be a much worse failure mode."""
    sync.N8N_ENCRYPTION_KEY = "totally_wrong_key"
    ciphertext = "U2FsdGVkX1+GCnOeCceRL5YG3P2WdgSi5o3aCVTCtvAkl4RUIyGFRw489kC2eCstTGSGeFxLwI3rGsJLBzC6ZA=="
    try:
        sync.decrypt_n8n_credential_data(ciphertext)
    except Exception:
        pass  # ValueError from cryptography is fine — padding fails.
    else:
        raise AssertionError("expected decryption to fail with wrong key")


if __name__ == "__main__":
    test_decrypt_known_vector()
    print("test_decrypt_known_vector PASS")
    test_decrypt_empty()
    print("test_decrypt_empty PASS")
    test_decrypt_no_key_raises()
    print("test_decrypt_no_key_raises PASS")
    test_decrypt_wrong_key_returns_garbage()
    print("test_decrypt_wrong_key_returns_garbage PASS")
