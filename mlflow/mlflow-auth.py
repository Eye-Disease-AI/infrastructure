import argparse
import configparser
import os
import sqlite3

from mlflow.server.auth.client import AuthServiceClient

AUTH_CONFIG_PATH = os.environ.get("MLFLOW_AUTH_CONFIG_PATH", "/mlauth/basic_auth.ini")
TRACKING_URI = os.environ.get("MLFLOW_TRACKING_URI", "http://localhost:5000")
DB_PATH = "/mlauth/users.db"

VALID_PERMISSIONS = {"READ", "USE", "EDIT", "MANAGE", "NO_PERMISSIONS"}


def load_auth():
    config = configparser.ConfigParser()
    config.read(AUTH_CONFIG_PATH)
    os.environ["MLFLOW_TRACKING_USERNAME"] = config["mlflow"]["admin_username"]
    os.environ["MLFLOW_TRACKING_PASSWORD"] = config["mlflow"]["admin_password"]


def cmd_list(args):
    auth_client = AuthServiceClient(TRACKING_URI)
    con = sqlite3.connect(DB_PATH)
    con.row_factory = sqlite3.Row

    users = con.execute("SELECT id, username, is_admin FROM users ORDER BY username").fetchall()
    if not users:
        print("No users found.")
        return

    for user in users:
        role = "admin" if user["is_admin"] else "user"
        print(f"\n{user['username']} ({role})")
        workspace_perms = auth_client.list_user_workspace_permissions(user["username"])
        if workspace_perms:
            for wp in workspace_perms:
                print(f"  {wp.workspace_name}: {wp.permission}")
        else:
            print("  (no workspace permissions)")

    con.close()


def cmd_create(args):
    client = AuthServiceClient(TRACKING_URI)
    client.create_user(args.username, args.password)
    if args.admin:
        client.update_user_admin(args.username, is_admin=True)
    print(f"Created user '{args.username}'" + (" (admin)" if args.admin else ""))


def cmd_passwd(args):
    client = AuthServiceClient(TRACKING_URI)
    client.update_user_password(args.username, args.new_password)
    print(f"Password updated for '{args.username}'")


def cmd_grant(args):
    client = AuthServiceClient(TRACKING_URI)
    client.set_workspace_permission(args.workspace, args.username, args.permission)
    print(f"Set '{args.permission}' for '{args.username}' on workspace '{args.workspace}'")


def cmd_revoke(args):
    client = AuthServiceClient(TRACKING_URI)
    client.delete_workspace_permission(args.workspace, args.username)
    print(f"Revoked workspace permission for '{args.username}' on workspace '{args.workspace}'")


parser = argparse.ArgumentParser(prog="mlflow-auth")
sub = parser.add_subparsers(dest="command", required=True)

sub.add_parser("list")

p = sub.add_parser("create")
p.add_argument("username")
p.add_argument("password")
p.add_argument("--admin", action="store_true")

p = sub.add_parser("passwd")
p.add_argument("username")
p.add_argument("new_password")

p = sub.add_parser("grant")
p.add_argument("username")
p.add_argument("workspace")
p.add_argument("permission", choices=sorted(VALID_PERMISSIONS))

p = sub.add_parser("revoke")
p.add_argument("username")
p.add_argument("workspace")

args = parser.parse_args()
load_auth()

{
    "list": cmd_list,
    "create": cmd_create,
    "passwd": cmd_passwd,
    "grant": cmd_grant,
    "revoke": cmd_revoke,
}[args.command](args)
