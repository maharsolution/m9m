"""Disposable local m9m instance and local tool runners for the parity harness.

Everything in this module works on throw-away state: a temporary SQLite
database in a temporary directory, a free loopback port and a process
that is always terminated again. The shared deployment is never touched.

The helpers are used by the C2 (roundtrip), C8 (fault injection), C9
(stability) and C10 (UI journey) contracts, and by the offline test
suite.
"""
from __future__ import annotations

import contextlib
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Iterator

ROOT = Path(__file__).resolve().parent.parent.parent
CACHE_DIR = ROOT / "tmp" / "parity-harness"
BINARY = CACHE_DIR / "m9m.exe"


def free_port() -> int:
    """Return a currently unused TCP port on the loopback interface."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _newest_source_mtime() -> float:
    newest = 0.0
    for folder in ("cmd", "internal"):
        base = ROOT / folder
        if not base.is_dir():
            continue
        for path in base.rglob("*.go"):
            try:
                newest = max(newest, path.stat().st_mtime)
            except OSError:
                continue
    return newest


def local_binary(rebuild: bool = False) -> Path:
    """Return the path of the harness-local m9m binary, building it if needed."""
    override = os.environ.get("M9M_LOCAL_BIN")
    if override:
        candidate = Path(override)
        if candidate.is_file():
            return candidate

    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    if not rebuild and BINARY.is_file():
        try:
            if BINARY.stat().st_mtime >= _newest_source_mtime():
                return BINARY
        except OSError:
            pass

    print(f"[localenv] building m9m -> {BINARY} (one-off, may take a minute)")
    completed = subprocess.run(
        ["go", "build", "-o", str(BINARY), "./cmd/m9m"],
        cwd=ROOT, capture_output=True, text=True,
        encoding="utf-8", errors="replace")
    if completed.returncode != 0:
        raise RuntimeError(
            "go build ./cmd/m9m failed:\n"
            f"{completed.stdout}\n{completed.stderr}")
    return BINARY


class LocalInstance:
    """A disposable m9m server bound to the loopback interface.

    Storage defaults to the in-memory backend: it needs no CGO (the
    Windows toolchain here has no C compiler, so the SQLite driver is a
    stub) and the instance is throw-away anyway. Set ``M9M_LOCAL_DB_TYPE``
    to override, e.g. to ``sqlite`` on a machine with a working cgo
    toolchain.
    """

    def __init__(self, metrics: bool = False, data_dir: Path | None = None,
                 db_type: str | None = None):
        self.metrics = metrics
        self.port = free_port()
        self.metrics_port = free_port() if metrics else 0
        self._data_dir = data_dir
        self._owns_dir = data_dir is None
        self.db_type = (db_type or os.environ.get("M9M_LOCAL_DB_TYPE") or "memory").lower()
        self.process: subprocess.Popen | None = None
        self.data_dir: Path | None = None

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.port}"

    @property
    def metrics_url(self) -> str:
        if not self.metrics_port:
            return ""
        return f"http://127.0.0.1:{self.metrics_port}"

    def start(self, timeout: float = 90.0) -> "LocalInstance":
        binary = local_binary()
        self.data_dir = self._data_dir or Path(tempfile.mkdtemp(prefix="m9m-parity-"))
        self.data_dir.mkdir(parents=True, exist_ok=True)

        command = [
            str(binary), "serve",
            "--host", "127.0.0.1",
            "--port", str(self.port),
            "--db-type", self.db_type,
            "--queue", "memory",
            "--workers", "1",
        ]
        if self.db_type == "sqlite":
            command += ["--db", str(self.data_dir / "m9m.db")]
        if self.metrics_port:
            command += ["--metrics-port", str(self.metrics_port)]

        stdout = open(self.data_dir / "server.out", "w", encoding="utf-8")
        stderr = open(self.data_dir / "server.err", "w", encoding="utf-8")
        self.process = subprocess.Popen(
            command, cwd=ROOT, stdout=stdout, stderr=stderr)

        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise RuntimeError(
                    "local m9m exited early:\n"
                    + (self.data_dir / "server.err").read_text(encoding="utf-8", errors="replace"))
            try:
                with urllib.request.urlopen(self.base_url + "/healthz", timeout=3) as response:
                    if response.status == 200:
                        return self
            except Exception:  # noqa: BLE001 - the server is still booting
                time.sleep(0.25)
        self.stop()
        raise RuntimeError("local m9m did not become ready in time")

    def stop(self) -> None:
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()
                with contextlib.suppress(subprocess.TimeoutExpired):
                    self.process.wait(timeout=5)
        self.process = None
        if self._owns_dir and self.data_dir and self.data_dir.exists():
            shutil.rmtree(self.data_dir, ignore_errors=True)

    def metrics_text(self) -> str:
        if not self.metrics_url:
            return ""
        try:
            with urllib.request.urlopen(self.metrics_url + "/metrics", timeout=10) as response:
                return response.read().decode("utf-8", errors="replace")
        except Exception:  # noqa: BLE001
            return ""

    def goroutines(self) -> int | None:
        """Read ``go_goroutines`` from the Prometheus endpoint."""
        for line in self.metrics_text().splitlines():
            if line.startswith("go_goroutines "):
                try:
                    return int(float(line.split()[1]))
                except (IndexError, ValueError):
                    return None
        return None


@contextlib.contextmanager
def local_instance(metrics: bool = False,
                   data_dir: Path | None = None) -> Iterator[LocalInstance]:
    """Context manager yielding a started disposable m9m instance."""
    instance = LocalInstance(metrics=metrics, data_dir=data_dir)
    try:
        instance.start()
        yield instance
    finally:
        instance.stop()


def run_command(command: list[str], cwd: Path | None = None,
                timeout: float = 900.0,
                env: dict[str, str] | None = None) -> dict[str, object]:
    """Run a command and capture its output. Never raises on a non-zero exit."""
    merged = dict(os.environ)
    if env:
        merged.update(env)
    try:
        completed = subprocess.run(
            command, cwd=str(cwd or ROOT), capture_output=True, text=True,
            encoding="utf-8", errors="replace",
            timeout=timeout, env=merged)
    except FileNotFoundError as exc:
        return {"exit_code": 127, "output": str(exc), "command": command}
    except subprocess.TimeoutExpired as exc:
        return {"exit_code": 124, "output": f"timeout after {timeout}s: {exc}",
                "command": command}
    output = (completed.stdout or "") + (completed.stderr or "")
    return {"exit_code": completed.returncode, "output": output.strip(),
            "command": command}


def run_go_test(package: str, run: str | None = None,
                timeout: float = 900.0) -> dict[str, object]:
    """Run ``go test`` for one package and report whether tests really ran."""
    command = ["go", "test", "-count=1"]
    if run:
        command += ["-run", run]
    command.append(package)
    result = run_command(command, timeout=timeout)
    output = str(result["output"])
    ran = ("ok " in output or "PASS" in output) and "no test files" not in output
    if run and "[no tests to run]" in output:
        ran = False
    result["tests_ran"] = ran
    return result


def _esbuild_binary() -> Path | None:
    """Return a runnable esbuild binary for this platform, if present.

    ``node_modules/esbuild/bin/esbuild`` may be a binary for a foreign
    platform (the workspace here was installed on Linux), so the
    platform specific package is preferred.
    """
    web = ROOT / "web"
    names = {
        "win32-x64": "esbuild.exe",
        "linux-x64": "esbuild",
        "linux-arm64": "esbuild",
        "darwin-arm64": "esbuild",
        "darwin-x64": "esbuild",
    }
    machine = os.uname().machine if hasattr(os, "uname") else ""
    platform_key = {
        "Windows": "win32-x64",
        "Linux": "linux-arm64" if "aarch64" in machine.lower() else "linux-x64",
        "Darwin": "darwin-arm64" if machine.lower() in ("arm64", "aarch64") else "darwin-x64",
    }.get(os.name if os.name != "nt" else "Windows", "linux-x64")
    candidates = [web / "node_modules" / "@esbuild" / platform_key / names[platform_key]]
    candidates += [web / "node_modules" / "@esbuild" / key / names[key] for key in names]
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    return None


def run_self_contained_expression_test(timeout: float = 600.0) -> dict[str, object]:
    """Bundle and run the self-contained browser expression test.

    ``web/src/components/workflow/parameters/expression/expression.test.ts``
    is a plain script (not a vitest suite): it bundles itself with
    esbuild and runs under Node.js. Vitest refuses to collect it ("No
    test suite found in file"), so the harness runs it the way its own
    header documents.
    """
    web = ROOT / "web"
    source = web / "src" / "components" / "workflow" / "parameters" / "expression" / "expression.test.ts"
    if not source.is_file():
        return {"exit_code": 127, "output": "expression.test.ts is missing",
                "tests_ran": False, "command": []}
    esbuild = _esbuild_binary()
    node = shutil.which("node.exe") or shutil.which("node")
    if esbuild is None or node is None:
        return {"exit_code": 127, "output": "esbuild or node is not available",
                "tests_ran": False, "command": []}
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    bundle = CACHE_DIR / "m9m-expression-test.cjs"
    build = run_command([str(esbuild), str(source), "--bundle", "--platform=node",
                         "--format=cjs", f"--outfile={bundle}"],
                        cwd=web, timeout=timeout)
    if build["exit_code"] != 0:
        return {"exit_code": build["exit_code"],
                "output": f"esbuild failed:\n{build['output']}",
                "tests_ran": False, "command": build["command"]}
    run = run_command([node, str(bundle)], cwd=web, timeout=timeout)
    run["tests_ran"] = "tests passed" in str(run["output"])
    return run


def run_web_unit_tests(timeout: float = 900.0) -> dict[str, object]:
    """Run every web test suite the repository ships.

    Three runners are involved:

    * ``vitest`` for the ``*.test.ts`` / ``*.test.mjs`` suites,
    * ``node --test`` for the ``*.node.test.mjs`` suites,
    * the self-contained expression script (esbuild bundle + node).
    """
    web = ROOT / "web"
    if not (web / "package.json").is_file():
        return {"exit_code": 127, "output": "web/package.json missing",
                "tests_ran": False, "command": []}
    npm = shutil.which("npm.cmd") or shutil.which("npm")
    if not npm:
        return {"exit_code": 127, "output": "npm is not installed",
                "tests_ran": False, "command": []}

    steps: list[tuple[str, dict[str, object]]] = []
    # The self-contained expression script is not a vitest suite; vitest
    # aborts the whole run with "No test suite found in file" when it is
    # collected, so it is excluded here and run separately below.
    steps.append(("vitest", run_command(
        [npm, "run", "test", "--", "--exclude", "**/expression.test.ts"],
        cwd=web, timeout=timeout)))
    steps.append(("node --test", run_command(
        [npm, "run", "test:node"], cwd=web, timeout=timeout)))
    steps.append(("expression script", run_self_contained_expression_test(timeout)))

    failed = [name for name, step in steps if step["exit_code"] != 0]
    output = "\n".join(
        f"[{name}] exit={step['exit_code']}\n{step['output']}" for name, step in steps)
    ran = all(bool(step.get("tests_ran", True)) for name, step in steps
              if name != "expression script" or step["exit_code"] == 0)
    return {"exit_code": 1 if failed else 0, "output": output,
            "tests_ran": bool(ran), "failed_steps": failed, "command": []}


if __name__ == "__main__":
    print(local_binary())
    sys.exit(0)
