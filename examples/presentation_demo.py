import subprocess
for intent in ["extract foo.tar.gz to /tmp", "list kube-system pods"]:
    result = subprocess.run(["go", "run", "./cmd/sh1llwrt", "ask", "--mock", "--shell", "bash", "--cwd", "/demo", "--json", intent], text=True, capture_output=True, check=True)
    print(result.stdout.strip())
