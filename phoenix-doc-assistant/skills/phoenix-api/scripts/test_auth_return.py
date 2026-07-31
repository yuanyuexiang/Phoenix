#!/usr/bin/env python3
"""登录完成返回 WorkBuddy 的标准库单元测试。"""
import json
import os
import tempfile
import unittest
from unittest import mock

import auth
import config


class ConfigCompatibilityTest(unittest.TestCase):
    def test_old_config_gets_default_return_scheme(self):
        with tempfile.TemporaryDirectory() as directory:
            path = os.path.join(directory, '.config.json')
            with open(path, 'w', encoding='utf-8') as stream:
                json.dump({'api_base_url': 'https://example.test', 'tokens': {}}, stream)
            with mock.patch.object(config, 'CONFIG_FILE', path):
                loaded = config.load_config()
            self.assertEqual(loaded['return_scheme'], 'workbuddy://')
            self.assertTrue(loaded['verify_ssl'])


class WorkBuddyActivationTest(unittest.TestCase):
    @mock.patch.object(auth.subprocess, 'Popen')
    @mock.patch.object(auth.platform, 'system', return_value='Darwin')
    def test_macos_opens_registered_scheme(self, _system, popen):
        self.assertTrue(auth._activate_workbuddy({'return_scheme': 'workbuddy://'}))
        self.assertEqual(popen.call_args.args[0], ['open', 'workbuddy://'])

    def test_rejects_unexpected_scheme(self):
        self.assertFalse(auth._activate_workbuddy({'return_scheme': 'https://attacker.invalid'}))

    def test_success_page_keeps_manual_fallback(self):
        page = auth._success_html({'return_scheme': 'workbuddy://'})
        self.assertIn('href="workbuddy://"', page)
        self.assertIn('返回 WorkBuddy', page)


if __name__ == '__main__':
    unittest.main()
