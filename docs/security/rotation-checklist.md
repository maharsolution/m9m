# Secret rotation checklist

Status: **DOCUMENT ONLY - the rotation below has NOT been executed.**
Owner: _unassigned_ · Date: _unassigned_

This checklist covers the two secrets that were exposed in the working
tree / evidence files and therefore have to be rotated:

| # | Secret | Where it lived | Blast radius |
|---|--------|----------------|--------------|
| 1 | MongoDB Atlas password | credential `data` in the parity evidence files (`tmp/parity-results/**/*.json`, already redacted in place) and in the stored credential "MongoDB account" | every workflow that uses the **"MongoDB account"** credential |
| 2 | Aiven Kafka password | same origin | every workflow that uses the **"Kafka - Aiven"** credential |

!!! warning "Accepted risk - the secrets are STILL VALID"
    The exposure has been closed on the m9m side (the evidence files are
    redacted in place, the API no longer hands out credential secrets
    without an explicit grant). **The secrets themselves have not been
    changed.** Until the rotation below runs, anyone who still holds the
    old values can authenticate against MongoDB Atlas and Aiven Kafka
    directly - the provider side does not know about the m9m API at all.
    This risk is accepted for now and tracked by this document.

## 0. Before you start

- [ ] Owner assigned (the person who runs the rotation and signs it off)
- [ ] Date scheduled
- [ ] A maintenance window agreed - the workflows that use the two
      credentials will fail while the old password is retired
- [ ] A backup of the current credential envelopes taken **through the
      authorized path** (the m9m API with an admin session, or the
      provider console) - never by copying a raw `data` blob into a
      ticket, a log or a spreadsheet
- [ ] The list of affected workflows exported (`GET /api/v1/workflows`
      with an admin session, then filter on the credential names)

## 1. MongoDB Atlas password

1. [ ] Open the MongoDB Atlas project that owns the database behind the
      **"MongoDB account"** credential
2. [ ] Create a **new** database user (do NOT rotate in place - a second
      user keeps the old workflows alive during the window)
3. [ ] Grant exactly the roles the workflows need (least privilege -
      `readWrite` on the application database, not `readWriteAnyDatabase`
      unless that is genuinely required)
4. [ ] Add the new user to the allow-list of the network access list
      (Atlas IP allow-list), so the new credential is not rejected for
      network reasons
5. [ ] Re-point the stored credential **atomically**:
      - `PUT /api/v1/credentials/<id>` with the new `data`
      - the envelope keeps the same `id` and `name`, so every workflow
        that references it keeps working
      - verify with `POST /api/v1/credentials/<id>/test` before the old
        user is retired
6. [ ] Run the workflows that use **"MongoDB account"** once - a manual
      execution is enough
7. [ ] Only now revoke the OLD database user in Atlas
8. [ ] Record the date of the revocation and the operator in the log
      section below

## 2. Aiven Kafka password

1. [ ] Open the Aiven service that backs the **"Kafka - Aiven"**
      credential
2. [ ] Create a new user with the same ACLs as the old one (the Kafka
      topic ACLs live in the Aiven project settings, not in the
      credential)
3. [ ] Re-point the stored credential **atomically**:
      - `PUT /api/v1/credentials/<id>` with the new `data`
      - same `id`, same `name`, same `type`
4. [ ] Verify with `POST /api/v1/credentials/<id>/test` and then with one
      real workflow execution against the Kafka node
5. [ ] Only now revoke the OLD user in Aiven
6. [ ] Record the date of the revocation and the operator in the log
      section below

## 3. Post-rotation checks

- [ ] `GET /api/v1/credentials/<id>` with an **admin session** returns the
      new envelope (and only then - a viewer or a plain automation key
      must get the envelope without `data`)
- [ ] `GET /api/v1/credentials/export` with an admin session still
      returns the **encrypted** payload (never plaintext) and writes an
      audit entry
- [ ] The parity evidence files under `tmp/parity-results/` carry no
      secret value (the credential `data` blobs are `[REDACTED]`)
- [ ] No secret was written to a log, a spreadsheet cell or a ticket
      during the rotation
- [ ] `M9M_JWT_SECRET` and the m9m API keys were considered separately -
      they are NOT rotated by this checklist (see
      `documentation/docs/api/authentication.md`)

## 4. Rotation log

| Date | Secret | New value stored | Old value revoked | Operator |
|------|--------|------------------|-------------------|----------|
| _YYYY-MM-DD_ | MongoDB Atlas password | _pending_ | _pending_ | _name_ |
| _YYYY-MM-DD_ | Aiven Kafka password | _pending_ | _pending_ | _name_ |

## 5. Rollback

If the rotation breaks a workflow:

1. [ ] Re-create the old user in the provider console (the old password
      is still known only to whoever holds it - if it does not, the
      rollback is "fix forward")
2. [ ] `PUT /api/v1/credentials/<id>` with the old `data`
3. [ ] Verify with `POST /api/v1/credentials/<id>/test` and one workflow
      execution
4. [ ] Record the rollback in the log table above

!!! danger "Never roll back the m9m API auth"
    `M9M_AUTH_ENABLED=false` reopens the whole API. It is the emergency
    switch of last resort, it must be used only with the instance behind
    a firewall, and it has to be reverted immediately.
