#!/usr/bin/env python3
"""
Development environment management script for sonic-change-agent.

Provides CLI interface for manual testing and debugging operations.
"""

import sys
import os
import argparse

# Add test/lib to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'lib'))

from environment import TestEnvironment


def main():
    parser = argparse.ArgumentParser(description='Manage sonic-change-agent development environment')
    
    subparsers = parser.add_subparsers(dest='command', help='Available commands')
    
    # Setup command
    setup_parser = subparsers.add_parser('setup', help='Setup complete development environment')
    setup_parser.add_argument('--skip-build', action='store_true', 
                             help='Skip Docker image build if image already exists')
    
    # Deploy command
    deploy_parser = subparsers.add_parser('deploy', help='Deploy/redeploy sonic-change-agent')
    deploy_parser.add_argument('--rebuild', action='store_true', 
                              help='Force rebuild of Docker image')
    
    # Device command
    device_parser = subparsers.add_parser('device', help='Create test NetworkDevice')
    device_parser.add_argument('name', help='Device name')
    device_parser.add_argument('--operation', default='OSUpgrade', help='Operation type')
    device_parser.add_argument('--action', default='PreloadImage', help='Operation action')
    device_parser.add_argument('--os-version', default='202505.01', help='OS version')
    device_parser.add_argument('--firmware-profile', default='SONiC-Test-Profile', help='Firmware profile')
    
    # Status command
    subparsers.add_parser('status', help='Show environment status')
    
    # Logs command
    logs_parser = subparsers.add_parser('logs', help='Collect logs for debugging')
    logs_parser.add_argument('test_name', help='Test/session name for log collection')
    
    # Cleanup command
    subparsers.add_parser('cleanup', help='Clean up development environment')
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        return 1
    
    env = TestEnvironment()
    
    try:
        if args.command == 'setup':
            print("🚀 Setting up complete development environment...")
            env.setup_cluster()
            env.build_image(skip_if_exists=args.skip_build)
            env.deploy_redis()
            env.deploy_agent()
            print("\n✅ Development environment ready!")
            print("\nNext steps:")
            print("  - Create test device: python test/scripts/dev-env.py device my-test-device")
            print("  - Check status: python test/scripts/dev-env.py status")
            print("  - Collect logs: python test/scripts/dev-env.py logs debug-session")
        
        elif args.command == 'deploy':
            print("🚀 Deploying sonic-change-agent...")
            if args.rebuild:
                env.build_image(skip_if_exists=False)
            else:
                env.build_image(skip_if_exists=True)
            env.deploy_agent()
            print("\n✅ Deployment completed!")
        
        elif args.command == 'device':
            print(f"📡 Creating NetworkDevice: {args.name}")
            env.create_device(
                args.name,
                operation=args.operation,
                operationAction=args.action,
                osVersion=args.os_version,
                firmwareProfile=args.firmware_profile
            )
        
        elif args.command == 'status':
            env.status()
        
        elif args.command == 'logs':
            log_dir = env.collect_logs(args.test_name)
            print(f"\n💡 Logs saved to: {log_dir}")
        
        elif args.command == 'cleanup':
            env.cleanup()
        
        return 0
        
    except Exception as e:
        print(f"\n❌ Error: {e}")
        return 1


if __name__ == '__main__':
    exit(main())