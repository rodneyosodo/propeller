#!/usr/bin/env python3
"""
Provision Atom resources for FL demo.

This script creates the necessary tenant, entities, and channel via Atom's GraphQL API,
then updates docker/.env with the provisioned credentials.

Usage:
    python3 examples/fl-demo/provision-smq.py

Requires:
    - requests library (pip install requests)
    - docker/.env file with ATOM_HTTP_PORT (default: 8080)
    - Atom service running and reachable

Environment variables (from docker/.env):
    ATOM_HTTP_PORT       - Atom HTTP port (default: 8080)
    ATOM_ADMIN_SECRET    - Admin password (default: 12345678)
"""

import json
import os
import re
import sys
import time
from pathlib import Path

import requests

# Default admin credentials
ADMIN_IDENTIFIER = "admin"
ADMIN_SECRET = "12345678"

# Demo configuration
TENANT_NAME = "fl-demo"
CHANNEL_NAME = "fl"
ENTITY_NAMES = [
    "manager",
    "proplet-1",
    "proplet-2",
    "proplet-3",
    "fl-coordinator",
    "proxy",
]


def load_env():
    """Load ATOM_HTTP_PORT from docker/.env if available."""
    repo_root = Path(__file__).parent.parent.parent
    env_file = repo_root / "docker" / ".env"
    if env_file.exists():
        for line in env_file.read_text().splitlines():
            line = line.strip()
            if line.startswith("ATOM_HTTP_PORT="):
                return line.split("=", 1)[1].strip()
            if line.startswith("ATOM_ADMIN_SECRET="):
                global ADMIN_SECRET
                ADMIN_SECRET = line.split("=", 1)[1].strip().strip('"')
    return "8080"


ATOM_HTTP_PORT = load_env()
ATOM_URL = f"http://localhost:{ATOM_HTTP_PORT}"
GRAPHQL_URL = f"{ATOM_URL}/graphql"


def gql(query, variables=None, token=None):
    """Execute a GraphQL query against Atom."""
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"

    payload = {"query": query}
    if variables:
        payload["variables"] = variables

    resp = requests.post(GRAPHQL_URL, json=payload, headers=headers, timeout=30)
    data = resp.json()

    if "errors" in data and data["errors"]:
        msgs = [e.get("message", str(e)) for e in data["errors"]]
        raise RuntimeError(f"GraphQL error: {'; '.join(msgs)}")

    return data.get("data", {})


def wait_for_service(url, name, max_retries=30):
    """Wait for a service to be available."""
    print(f"Waiting for {name} service...")
    for i in range(max_retries):
        try:
            resp = requests.get(f"{url}/health", timeout=2)
            if resp.status_code in [200, 404]:
                print(f"✓ {name} service is ready")
                return True
        except requests.exceptions.RequestException:
            pass
        time.sleep(1)
    print(f"✗ {name} service did not become available")
    return False


def login():
    """Login and get access token."""
    print("\n=== Logging in ===")
    payload = {
        "identifier": ADMIN_IDENTIFIER,
        "secret": ADMIN_SECRET,
        "kind": "password",
    }

    try:
        resp = requests.post(
            f"{ATOM_URL}/auth/login",
            json=payload,
            headers={"Content-Type": "application/json"},
            timeout=10,
        )
        resp.raise_for_status()
        data = resp.json()
        token = data.get("token")
        if not token:
            print(f"Error: No token in response: {data}")
            return None
        print("✓ Login successful")
        return token
    except requests.exceptions.RequestException as e:
        print(f"✗ Login failed: {e}")
        if hasattr(e, "response") and e.response is not None:
            print(f"  Response: {e.response.text}")
        return None


def get_action_ids(token):
    """Fetch publish and subscribe action IDs from Atom."""
    print("\n=== Fetching action IDs ===")
    query = """query {
        actions(limit:100,offset:0) {
            items { id name }
        }
    }"""

    try:
        data = gql(query, token=token)
        actions = data.get("actions", {}).get("items", [])
        action_map = {
            a["name"]: a["id"] for a in actions if a.get("id") and a.get("name")
        }

        result = {}
        for name in ["publish", "subscribe"]:
            aid = action_map.get(name)
            if not aid:
                print(f"✗ Action '{name}' not found on server")
                return None
            result[name] = aid

        print(
            f"✓ Found actions: publish={result['publish']}, subscribe={result['subscribe']}"
        )
        return result
    except Exception as e:
        print(f"✗ Failed to fetch actions: {e}")
        return None


def ensure_tenant(token):
    """Create or get tenant by name."""
    print(f"\n=== Ensuring tenant '{TENANT_NAME}' ===")

    # First, list existing tenants
    list_query = """query {
        tenants(limit:100) { items { id name } }
    }"""

    try:
        data = gql(list_query, token=token)
        for t in data.get("tenants", {}).get("items", []):
            if t.get("name") == TENANT_NAME or t.get("name") == TENANT_NAME:
                print(f"✓ Found existing tenant: {t['id']} (name: {t['name']})")
                return t["id"]
    except Exception:
        pass

    # Create new tenant
    create_query = """mutation($name:String!) {
        createTenant(input:{name:$name}) { id name }
    }"""

    try:
        data = gql(create_query, {"name": TENANT_NAME}, token=token)
        tenant = data.get("createTenant", {})
        print(f"✓ Tenant created: {tenant['id']} (name: {tenant['name']})")
        return tenant["id"]
    except Exception as e:
        print(f"✗ Failed to create tenant: {e}")
        return None


def create_service_entity(token, tenant_id, name):
    """Create a service entity in the tenant."""
    query = """mutation($tid:ID!,$name:String!) {
        createEntity(input:{kind:service,name:$name,tenantId:$tid,attributes:{}}) { id }
    }"""

    try:
        data = gql(query, {"tid": tenant_id, "name": name}, token=token)
        entity_id = data.get("createEntity", {}).get("id")
        if entity_id:
            print(f"✓ Entity created: {name} (ID: {entity_id})")
            return entity_id
        print(f"✗ Entity '{name}' returned empty id")
        return None
    except Exception as e:
        print(f"✗ Failed to create entity '{name}': {e}")
        return None


def create_api_key(token, entity_id, description):
    """Create an API key (access token) for an entity."""
    query = """mutation($eid:ID!,$desc:String!,$name:String!) {
        createAccessToken(input:{
            subjectId:$eid,
            name:$name,
            description:$desc,
            scoped:false,
            permissions:[]
        }){ credentialId token }
    }"""

    try:
        data = gql(
            query,
            {"eid": entity_id, "desc": description, "name": description},
            token=token,
        )
        key = data.get("createAccessToken", {})
        api_token = key.get("token")
        if api_token:
            print(f"✓ API key created for {description}")
            return api_token
        print(f"✗ API key for '{description}' returned empty token")
        return None
    except Exception as e:
        print(f"✗ Failed to create API key for '{description}': {e}")
        return None


def create_channel(token, tenant_id):
    """Create a channel resource."""
    print(f"\n=== Creating Channel '{CHANNEL_NAME}' ===")
    query = """mutation($tid:ID!,$name:String!) {
        createResource(input:{kind:"channel",name:$name,tenantId:$tid,attributes:{}}){ id }
    }"""

    try:
        data = gql(query, {"tid": tenant_id, "name": CHANNEL_NAME}, token=token)
        resource_id = data.get("createResource", {}).get("id")
        if resource_id:
            print(f"✓ Channel created: {resource_id}")
            return resource_id
        print("✗ Channel returned empty id")
        return None
    except Exception as e:
        print(f"✗ Failed to create channel: {e}")
        return None


def connect_entity(token, tenant_id, entity_id, channel_id, action_ids):
    """Connect an entity to a channel by creating permission block and direct policy."""
    # Step 1: Create permission block
    pb_query = """mutation($tid:ID!,$cid:ID!,$aid1:ID!,$aid2:ID!){
        createPermissionBlock(input:{
            tenantId:$tid,
            scopeMode:"object",
            objectKind:"resource",
            objectType:"resource:channel",
            objectId:$cid,
            effect:allow,
            actionIds:[$aid1,$aid2]
        }){id}
    }"""

    try:
        data = gql(
            pb_query,
            {
                "tid": tenant_id,
                "cid": channel_id,
                "aid1": action_ids["publish"],
                "aid2": action_ids["subscribe"],
            },
            token=token,
        )
        pb_id = data.get("createPermissionBlock", {}).get("id")
        if not pb_id:
            print(f"  ✗ Failed to create permission block")
            return False
    except Exception as e:
        print(f"  ✗ Failed to create permission block: {e}")
        return False

    # Step 2: Create direct policy
    dp_query = """mutation($tid:ID!,$sid:ID!,$pbid:ID!){
        createDirectPolicy(input:{
            tenantId:$tid,
            subjectKind:entity,
            subjectId:$sid,
            permissionBlockId:$pbid
        }){id}
    }"""

    try:
        data = gql(
            dp_query,
            {
                "tid": tenant_id,
                "sid": entity_id,
                "pbid": pb_id,
            },
            token=token,
        )
        dp_id = data.get("createDirectPolicy", {}).get("id")
        if dp_id:
            print(f"  ✓ Connected entity to channel (policy: {dp_id})")
            return True
        print(f"  ✗ Failed to create direct policy")
        return False
    except Exception as e:
        print(f"  ✗ Failed to create direct policy: {e}")
        return False


def update_env_file(env_file, tenant_id, channel_id, entities):
    """Update docker/.env file with provisioned credentials."""
    if not env_file.exists():
        print(f"Warning: .env file not found: {env_file}")
        return False

    content = env_file.read_text()
    original_content = content

    # Map entity names to environment variables
    env_mapping = {
        "manager": {
            "id_var": "MANAGER_ENTITY_ID",
            "key_var": "MANAGER_API_KEY",
            "tenant_var": "MANAGER_TENANT_ID",
            "channel_var": "MANAGER_CHANNEL_ID",
        },
        "proplet-1": {
            "id_var": "PROPLET_ENTITY_ID",
            "key_var": "PROPLET_API_KEY",
            "tenant_var": "PROPLET_TENANT_ID",
            "channel_var": "PROPLET_CHANNEL_ID",
        },
        "proplet-2": {
            "id_var": "PROPLET_2_ENTITY_ID",
            "key_var": "PROPLET_2_API_KEY",
        },
        "proplet-3": {
            "id_var": "PROPLET_3_ENTITY_ID",
            "key_var": "PROPLET_3_API_KEY",
        },
        "fl-coordinator": {
            "id_var": "COORDINATOR_ENTITY_ID",
            "key_var": "COORDINATOR_API_KEY",
        },
        "proxy": {
            "id_var": "PROXY_ENTITY_ID",
            "key_var": "PROXY_API_KEY",
            "tenant_var": "PROXY_TENANT_ID",
            "channel_var": "PROXY_CHANNEL_ID",
        },
    }

    def update_or_add_var(var_name, var_value):
        nonlocal content
        pattern = rf"^(\s*)(#?\s*)({re.escape(var_name)}\s*=\s*)([^\n]*)"
        lines = content.split("\n")
        found = False
        new_lines = []

        for line in lines:
            match = re.match(pattern, line)
            if match:
                found = True
                if match.group(2).strip().startswith("#"):
                    new_lines.append(f"{var_name}={var_value}")
                else:
                    new_lines.append(f"{match.group(1)}{match.group(3)}{var_value}")
            else:
                new_lines.append(line)

        if not found:
            if new_lines and new_lines[-1]:
                new_lines.append("")
            new_lines.append(f"{var_name}={var_value}")

        content = "\n".join(new_lines)

    # Update tenant and channel IDs for all sections
    tenant_vars = {
        "MANAGER_TENANT_ID": tenant_id,
        "PROPLET_TENANT_ID": tenant_id,
        "PROXY_TENANT_ID": tenant_id,
    }
    channel_vars = {
        "MANAGER_CHANNEL_ID": channel_id,
        "PROPLET_CHANNEL_ID": channel_id,
        "PROXY_CHANNEL_ID": channel_id,
    }

    for var_name, var_value in {**tenant_vars, **channel_vars}.items():
        update_or_add_var(var_name, var_value)

    # Update entity IDs and API keys
    for entity_name, info in entities.items():
        mapping = env_mapping.get(entity_name)
        if not mapping:
            continue

        eid = info.get("entity_id")
        key = info.get("api_key")

        if eid:
            update_or_add_var(mapping["id_var"], eid)
        if key:
            update_or_add_var(mapping["key_var"], key)

    if content != original_content:
        backup_path = env_file.with_suffix(".env.bak")
        if backup_path.exists():
            backup_path.unlink()
        env_file.rename(backup_path)
        print(f"  Created backup: {backup_path.name}")
        env_file.write_text(content)
        return True

    return False


def main():
    print("=" * 60)
    print("Atom Provisioning Script for FL Demo")
    print("=" * 60)
    print(f"Atom URL: {ATOM_URL}")

    # Wait for Atom service
    if not wait_for_service(ATOM_URL, "Atom"):
        sys.exit(1)

    # Login
    token = login()
    if not token:
        print("\n✗ Provisioning failed: Could not login")
        sys.exit(1)

    # Fetch action IDs (needed for connecting entities)
    action_ids = get_action_ids(token)
    if not action_ids:
        print("\n✗ Provisioning failed: Could not fetch action IDs")
        sys.exit(1)

    # Create or get tenant
    tenant_id = ensure_tenant(token)
    if not tenant_id:
        print("\n✗ Provisioning failed: Could not create tenant")
        sys.exit(1)

    # Create entities and API keys
    print("\n=== Creating Entities and API Keys ===")
    entities = {}
    for entity_name in ENTITY_NAMES:
        eid = create_service_entity(token, tenant_id, entity_name)
        if not eid:
            print(f"  ⚠ Warning: Could not create entity '{entity_name}'")
            continue

        # Determine description for API key based on entity
        api_desc = f"{entity_name}-mqtt"
        if entity_name == "fl-coordinator":
            api_desc = "fl-coordinator-mqtt"

        api_key = create_api_key(token, eid, api_desc)
        if not api_key:
            print(f"  ⚠ Warning: Could not create API key for '{entity_name}'")
            # Still add entity with empty key
            entities[entity_name] = {"entity_id": eid, "api_key": ""}
        else:
            entities[entity_name] = {"entity_id": eid, "api_key": api_key}

    if not entities:
        print("\n✗ Provisioning failed: No entities created")
        sys.exit(1)

    # Create channel
    channel_id = create_channel(token, tenant_id)
    if not channel_id:
        print("\n✗ Provisioning failed: Could not create channel")
        sys.exit(1)

    # Connect entities to channel
    print("\n=== Connecting Entities to Channel ===")
    connected = 0
    for entity_name, info in entities.items():
        eid = info.get("entity_id")
        if eid:
            print(f"  Connecting '{entity_name}'...")
            if connect_entity(token, tenant_id, eid, channel_id, action_ids):
                connected += 1

    # Print summary
    print("\n" + "=" * 60)
    print("Provisioning Summary")
    print("=" * 60)
    print(f"Tenant ID: {tenant_id}")
    print(f"Channel ID: {channel_id}")
    print(f"Entities connected: {connected}/{len(entities)}")
    print("\nEntities:")
    for name, info in entities.items():
        print(f"  {name}:")
        print(f"    ID: {info.get('entity_id', 'N/A')}")
        print(f"    API Key: {info.get('api_key', 'N/A')}")

    print("\n✓ Provisioning completed successfully!")

    # Update docker/.env with new credentials
    repo_root = Path(__file__).parent.parent.parent
    env_file = repo_root / "docker" / ".env"
    if update_env_file(env_file, tenant_id, channel_id, entities):
        print(f"\n✓ Updated {env_file} with new credentials")
    else:
        print(f"\n⚠ Could not update {env_file} automatically")
        print("   Please update it manually with the credentials shown above")

    print("\nNote: Recreate services to apply new credentials:")
    print(
        "  docker compose -f docker/compose.yaml -f docker/compose.propeller.yaml -f examples/fl-demo/compose.yaml --env-file docker/.env up -d --force-recreate"
    )

    # Also write a config.toml for reference
    config_path = repo_root / "config.toml"
    with open(config_path, "w") as f:
        f.write("# Propeller Configuration\n")
        f.write("# Provisioned by examples/fl-demo/provision-smq.py\n\n")
        f.write(f"[manager]\n")
        f.write(f'tenant_id = "{tenant_id}"\n')
        f.write(f'entity_id = "{entities.get("manager", {}).get("entity_id", "")}"\n')
        f.write(f'api_key = "{entities.get("manager", {}).get("api_key", "")}"\n')
        f.write(f'channel_id = "{channel_id}"\n\n')
        f.write(f"[proplet]\n")
        f.write(f'tenant_id = "{tenant_id}"\n')
        f.write(f'entity_id = "{entities.get("proplet-1", {}).get("entity_id", "")}"\n')
        f.write(f'api_key = "{entities.get("proplet-1", {}).get("api_key", "")}"\n')
        f.write(f'channel_id = "{channel_id}"\n\n')
        f.write(f"[proxy]\n")
        f.write(f'tenant_id = "{tenant_id}"\n')
        f.write(f'entity_id = "{entities.get("proxy", {}).get("entity_id", "")}"\n')
        f.write(f'api_key = "{entities.get("proxy", {}).get("api_key", "")}"\n')
        f.write(f'channel_id = "{channel_id}"\n')
    print(f"\n✓ Written {config_path}")


if __name__ == "__main__":
    main()
