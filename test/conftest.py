"""
pytest configuration and fixtures for sonic-change-agent integration tests.

Provides setup/teardown for cluster, image, and deployment using TestEnvironment class.
"""

import pytest
import os
import sys

# Add test/lib to path
test_lib_path = os.path.join(os.path.dirname(__file__), 'lib')
if test_lib_path not in sys.path:
    sys.path.insert(0, test_lib_path)

from environment import TestEnvironment, kubectl


# Global test environment instance
_test_env = None


def pytest_addoption(parser):
    """Add custom pytest options."""
    parser.addoption(
        "--reuse-env", action="store_true", default=False,
        help="Reuse existing test environment instead of creating new one"
    )
    parser.addoption(
        "--dry-run", action="store_true", default=True,
        help="Run tests in DRY_RUN mode (default: True)"
    )


@pytest.fixture(scope="session")
def cluster(request):
    """Create and manage test cluster lifecycle."""
    global _test_env
    
    if _test_env is None:
        _test_env = TestEnvironment()
    
    reuse_env = request.config.getoption("--reuse-env")
    if not reuse_env:
        try:
            _test_env.setup_cluster()
        except Exception as e:
            pytest.fail(f"Failed to setup cluster: {e}")
    
    yield _test_env.cluster_name
    
    # Cleanup is handled by session teardown


@pytest.fixture(scope="session")
def docker_image(request):
    """Build test Docker image."""
    global _test_env
    
    if _test_env is None:
        _test_env = TestEnvironment()
    
    reuse_env = request.config.getoption("--reuse-env")
    skip_build = reuse_env or bool(os.getenv("SKIP_DOCKER_BUILD"))
    
    try:
        _test_env.build_image(skip_if_exists=skip_build)
    except Exception as e:
        pytest.fail(f"Failed to build image: {e}")
    
    yield _test_env.image_name


@pytest.fixture(scope="session") 
def redis_deployment(cluster):
    """Deploy Redis with CONFIG_DB."""
    global _test_env
    
    try:
        _test_env.deploy_redis()
    except Exception as e:
        pytest.fail(f"Failed to deploy Redis: {e}")
    
    yield "redis"


@pytest.fixture(scope="session")
def dry_run_mode(request):
    """Get dry run mode from pytest options."""
    return request.config.getoption("--dry-run")


@pytest.fixture(scope="session")
def sonic_deployment(cluster, docker_image, redis_deployment, dry_run_mode):
    """Deploy sonic-change-agent with configurable DRY_RUN mode."""
    global _test_env
    
    try:
        _test_env.deploy_agent(dry_run=dry_run_mode)
    except Exception as e:
        pytest.fail(f"Failed to deploy sonic-change-agent: {e}")
    
    yield "sonic-change-agent"


@pytest.fixture
def network_device():
    """Factory to create and cleanup NetworkDevice resources."""
    global _test_env
    
    def _create_device(name, **spec_kwargs):
        try:
            return _test_env.create_device(name, **spec_kwargs)
        except Exception as e:
            pytest.fail(f"Failed to create NetworkDevice {name}: {e}")
    
    yield _create_device
    
    # Cleanup is handled by TestEnvironment


@pytest.fixture(autouse=True)
def auto_collect_logs(request, sonic_deployment):
    """Automatically collect logs after each test."""
    yield  # Run the test
    
    # Collect logs after test completion
    global _test_env
    if _test_env is not None:
        test_name = request.node.name
        log_dir = _test_env.collect_logs(test_name)
        
        # Add log directory to test report
        if hasattr(request.node, 'user_properties'):
            request.node.user_properties.append(("log_directory", log_dir))


def pytest_configure(config):
    """Configure pytest with custom markers."""
    config.addinivalue_line("markers", "workflow: marks tests that validate workflow execution")
    config.addinivalue_line("markers", "slow: marks tests as slow")


def pytest_runtest_makereport(item, call):
    """Add log collection info to test reports."""
    if call.when == "call":
        log_dir = None
        for name, value in getattr(item, 'user_properties', []):
            if name == "log_directory":
                log_dir = value
                break
        
        if log_dir:
            print(f"\n📋 Test logs saved to: {log_dir}")
            
            # If test failed, show quick summary
            if call.excinfo is not None:
                summary_file = os.path.join(log_dir, "README.txt")
                if os.path.exists(summary_file):
                    print(f"💡 For debugging, check logs in: {log_dir}")


def pytest_sessionfinish(session, exitstatus):
    """Clean up test environment at session end."""
    global _test_env
    
    # Only cleanup if we don't want to reuse the environment
    reuse_env = session.config.getoption("--reuse-env")
    
    if _test_env is not None and not reuse_env:
        print("\n🧹 Cleaning up test environment...")
        try:
            _test_env.cleanup()
        except Exception as e:
            print(f"Warning: Cleanup failed: {e}")