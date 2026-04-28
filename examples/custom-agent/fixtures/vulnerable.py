import sqlite3
import sys


def get_user(user_id):
    """Fetch a user from the database by ID — VULNERABLE to SQL injection."""
    conn = sqlite3.connect("app.db")
    cursor = conn.cursor()

    # BAD: string concatenation allows SQL injection
    query = "SELECT * FROM users WHERE id = " + user_id
    cursor.execute(query)

    row = cursor.fetchone()
    conn.close()
    return row


def search_users(name):
    """Search users by name — VULNERABLE to SQL injection."""
    conn = sqlite3.connect("app.db")
    cursor = conn.cursor()

    # BAD: f-string interpolation in SQL query
    cursor.execute(f"SELECT * FROM users WHERE name LIKE '%{name}%'")

    rows = cursor.fetchall()
    conn.close()
    return rows


if __name__ == "__main__":
    uid = sys.argv[1] if len(sys.argv) > 1 else "1"
    print(get_user(uid))
