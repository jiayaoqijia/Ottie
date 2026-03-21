"""Test Snapshot GraphQL API for Lido governance proposals."""
import json
import urllib.request

query = json.dumps({
    "query": '{proposals(first:3,where:{space_in:["lido-snapshot.eth"]},orderBy:"created",orderDirection:desc){id title state created}}'
})

req = urllib.request.Request(
    "https://hub.snapshot.org/graphql",
    data=query.encode(),
    headers={"Content-Type": "application/json", "User-Agent": "Ottie/1.0"},
)
resp = json.loads(urllib.request.urlopen(req, timeout=15).read())
proposals = resp["data"]["proposals"]
assert len(proposals) > 0, "No proposals found"
for p in proposals:
    print(f'  [{p["state"]}] {p["title"][:60]}')
print(f"{len(proposals)} governance proposals fetched")
print("GOVERNANCE_OK")
