import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nectar_apps_ui/api/models.dart';
import 'package:nectar_apps_ui/api/session.dart';
import 'package:nectar_apps_ui/core/client_version.dart';
import 'package:nectar_apps_ui/core/format.dart';
import 'package:nectar_apps_ui/core/server_config.dart';
import 'package:nectar_apps_ui/features/apps/artifact_file.dart';
import 'package:nectar_apps_ui/features/apps/resources.dart';
import 'package:nectar_apps_ui/features/projects/new_project_dialog.dart';
import 'package:nectar_apps_ui/router.dart';
import 'package:nectar_apps_ui/theme/theme.dart';
import 'package:nectar_apps_ui/widgets/gauge.dart';
import 'package:nectar_apps_ui/widgets/hex.dart';
import 'package:nectar_apps_ui/widgets/status.dart';

void main() {
  group('sessionRedirect', () {
    String? r(SessionStatus st, String url) {
      final uri = Uri.parse(url);
      return sessionRedirect(SessionState(status: st), uri.path, uri);
    }

    test('loading keeps the destination', () {
      expect(
        r(SessionStatus.loading, '/projects/3'),
        '/splash?from=%2Fprojects%2F3',
      );
      expect(r(SessionStatus.loading, '/'), '/splash');
    });
    test('signed out goes to login, then back', () {
      expect(
        r(SessionStatus.signedOut, '/splash?from=%2Fprojects%2F3'),
        '/login?from=%2Fprojects%2F3',
      );
      expect(
        r(SessionStatus.signedIn, '/login?from=%2Fprojects%2F3'),
        '/projects/3',
      );
      expect(r(SessionStatus.signedIn, '/login'), '/');
      expect(r(SessionStatus.signedIn, '/projects'), isNull);
    });
    test('setup link keeps its token', () {
      final to = r(SessionStatus.loading, '/setup?token=abc')!;
      expect(to.startsWith('/splash?from='), isTrue);
      final setup = r(SessionStatus.needsSetup, to)!;
      expect(setupTokenFrom(Uri.parse(setup)), 'abc');
      expect(r(SessionStatus.needsSetup, '/setup?token=abc'), isNull);
      // after setup the user lands on the dashboard, not back on /setup
      expect(r(SessionStatus.signedIn, setup), '/');
    });
    test('no open redirects', () {
      expect(
        r(SessionStatus.signedIn, '/login?from=https%3A%2F%2Fevil.example'),
        '/',
      );
    });
  });

  test('slugify', () {
    expect(slugify('My Shop!'), 'my-shop');
    expect(slugify('  --Über Café 2--  '), 'ber-caf-2');
    expect(slugify('a' * 40).length, 30);
  });

  test('server address normalization', () {
    expect(
      ServerConfig.normalize('panel.example.com/'),
      'https://panel.example.com',
    );
    expect(
      ServerConfig.normalize('http://localhost:8480/api/'),
      'http://localhost:8480',
    );
  });

  test('formatting', () {
    expect(formatCpu(250), '250m');
    expect(formatCpu(1000), '1 core');
    expect(formatCpu(1500), '1.5 cores');
    expect(formatMb(512), '512 MB');
    expect(formatBytes(4 * 1024 * 1024 * 1024), '4.0 GB');
    final now = DateTime(2026, 10, 20, 12);
    expect(
      timeAgo(now.subtract(const Duration(minutes: 5)), now: now),
      '5 min ago',
    );
  });

  test('permissions', () {
    final p = Permissions.fromJson({
      'owner': false,
      'global': ['server.view'],
      'projects': {
        '3': ['app.deploy'],
      },
    });
    expect(p.can('server.view'), isTrue);
    expect(p.can('app.deploy'), isFalse);
    expect(p.can('app.deploy', projectId: 3), isTrue);
    expect(Permissions.fromJson({'owner': true}).can('anything'), isTrue);
  });

  test('.env parsing', () {
    final vars = parseDotEnv('''
# comment
export DATABASE_URL="postgres://u:p@db/x"
LOG_LEVEL=info # trailing comment
EMPTY=
bad key=1
MSG='a # b'
LOG_LEVEL=debug
''');
    expect(vars, [
      ('DATABASE_URL', 'postgres://u:p@db/x'),
      ('EMPTY', ''),
      ('MSG', 'a # b'),
      ('LOG_LEVEL', 'debug'),
    ]);
    expect(looksSecret('DATABASE_URL'), isTrue);
    expect(looksSecret('LOG_LEVEL'), isFalse);
  });

  test('resource presets', () {
    expect(presetFor(500, 512)?.label, 'Small');
    expect(presetFor(300, 512), isNull);
  });

  test('artifact types', () {
    Runtime rt(List<String> accept) => Runtime(
      id: 'x',
      name: 'X',
      description: '',
      kinds: const ['web'],
      artifactKind: 'file',
      accept: accept,
      versions: const [],
      port: 8080,
      healthType: 'tcp',
      healthPath: '',
    );
    expect(pickerExtensions(rt(['.zip', '.tar.gz', '.tgz'])), [
      'zip',
      'gz',
      'tgz',
    ]);
    expect(pickerExtensions(rt(['*'])), isNull);
    expect(artifactProblem(rt(['.jar']), 'app.JAR'), isNull);
    expect(artifactProblem(rt(['.jar']), 'app.zip'), isNotNull);
    expect(artifactProblem(rt(['.zip', '.tar.gz']), 'site.tar.gz'), isNull);
  });

  test('status tones', () {
    expect(toneFor('running'), StatusTone.running);
    expect(toneFor('degraded'), StatusTone.warning);
    expect(toneFor('failed'), StatusTone.failed);
    expect(toneFor('stopped'), StatusTone.stopped);
    expect(statusLabel('new'), 'Not deployed');
  });

  testWidgets('components render in both themes', (tester) async {
    for (final b in Brightness.values) {
      await tester.pumpWidget(
        MaterialApp(
          theme: buildNectarTheme(b),
          home: const Scaffold(
            body: Column(
              children: [
                StatusPill(status: 'running'),
                StatusPill(status: 'deploying'),
                RingGauge(value: 0.42, label: 'CPU'),
                RingGauge(value: 0, label: 'Memory', unavailable: true),
                NectarMark(size: 40),
                HexBadge(child: Text('A')),
              ],
            ),
          ),
        ),
      );
      await tester.pump(const Duration(seconds: 1));
      expect(find.text('Running'), findsOneWidget);
      expect(find.text('—'), findsOneWidget);
    }
  });

  group('updates', () {
    test('clientVersion matches pubspec.yaml', () {
      final m = RegExp(
        r'^version:\s*([0-9.]+)',
        multiLine: true,
      ).firstMatch(File('pubspec.yaml').readAsStringSync());
      expect(m?.group(1), clientVersion);
    });

    test('compareVersions', () {
      expect(compareVersions('0.1.0', '0.2.0'), -1);
      expect(compareVersions('v1.2.3', '1.2.3'), 0);
      expect(compareVersions('1.2.3-beta.1', '1.2.3'), -1);
      expect(compareVersions('1.10.0', '1.9.9'), 1);
      expect(compareVersions('dev', '1.0.0'), 0);
    });

    test('UpdatesOverview parses the API answer', () {
      final o = UpdatesOverview.fromJson({
        'settings': {
          'panel': {'policy': 'notify', 'channel': 'stable'},
        },
        'state': {
          'checked_at': '2026-10-28T12:00:00Z',
          'components': {
            'panel': {
              'installed': {'version': '0.6.0'},
              'available': [
                {
                  'component': 'panel',
                  'version': '0.7.0',
                  'kind': 'minor',
                  'migrations': true,
                  'notes': '- feat: x',
                },
              ],
              'waiting': '',
            },
            'k3s': {
              'installed': {'version': 'v1.31.4+k3s1'},
              'extra': {
                'controller': true,
                'nodes': [
                  {
                    'name': 'n1',
                    'version': 'v1.31.4+k3s1',
                    'control_plane': true,
                  },
                ],
              },
            },
          },
        },
        'bad': {'gateway@1.3.0': 'canary failed'},
        'runs': [
          {
            'id': 2,
            'trigger': 'manual',
            'state': 'running',
            'summary': 'panel 0.6.0 → 0.7.0',
            'plan': {
              'steps': [
                {'component': 'panel', 'from': '0.6.0', 'to': '0.7.0'},
              ],
            },
            'status': {},
            'started_at': '2026-10-28T12:01:00Z',
          },
        ],
        'active': {
          'id': 2,
          'state': 'running',
          'status': {
            'steps': [
              {
                'component': 'panel',
                'from': '0.6.0',
                'to': '0.7.0',
                'state': 'running',
              },
            ],
            'log': [
              {'t': '2026-10-28T12:01:01Z', 'm': 'go'},
            ],
          },
        },
        'window_open': true,
      });
      expect(o.component('panel').hasUpdate, isTrue);
      expect(o.component('panel').available.first.migrations, isTrue);
      expect(o.component('k3s').controller, isTrue);
      expect(o.component('k3s').nodes.single.controlPlane, isTrue);
      expect(o.component('gateway').installed, '');
      expect(o.bad['gateway@1.3.0'], 'canary failed');
      expect(o.runs.single.steps.single.to, '0.7.0');
      expect(o.runs.single.active, isTrue);
      expect(o.active!.steps.single.state, 'running');
      expect(o.active!.log.single.$2, 'go');
    });
  });

  group('access and gateway models', () {
    test('ApiTokenInfo', () {
      final t = ApiTokenInfo.fromJson({
        'id': 3,
        'name': 'CI',
        'role_id': 4,
        'role_name': 'Deployer',
        'project_id': 1,
        'project_name': 'Shop',
        'expires_at': '2020-01-01T00:00:00Z',
      });
      expect(t.expired, isTrue);
      expect(t.roleName, 'Deployer');
      expect(ApiTokenInfo.fromJson({'id': 1, 'name': 'x'}).expired, isFalse);
    });

    test('GatewayRoute', () {
      final r = GatewayRoute.fromJson({
        'id': 7,
        'domain': 'shop.example.com',
        'path_prefix': '/api',
        'mode': 'proxy',
        'target_ip': 'api.prj-shop.svc.cluster.local',
        'target_port': 8080,
        'rate_limit': 20,
        'cert_not_after': 1893456000,
        'managed_by': 'nectar-apps',
      });
      expect(r.label, 'shop.example.com/api');
      expect(r.target, 'api.prj-shop.svc.cluster.local:8080');
      expect(r.rateLimit, 20);
      expect(r.certNotAfter!.toUtc().year, 2030);
      final h = RouteHealth.fromJson({'up': false, 'last_error': 'timeout'});
      expect(h.up, isFalse);
      expect(RouteHealth.fromJson({}).up, isNull);
    });

    test('ProjectMember and roles', () {
      final m = ProjectMember.fromJson({
        'user_id': 2,
        'email': 'd@x.io',
        'roles': [
          {'role_id': 5, 'role_name': 'Developer', 'project_id': 1},
        ],
        'global_roles': [
          {'role_id': 6, 'role_name': 'Viewer'},
        ],
      });
      expect(m.roles.single.projectId, 1);
      expect(m.globalRoles.single.projectId, isNull);
      expect(
        const RoleBindingInfo(
          roleId: 5,
          roleName: '',
          projectId: null,
        ).toJson(),
        {'role_id': 5, 'project_id': null},
      );
      final g = PermissionGroup.fromJson({
        'group': 'Apps',
        'permissions': [
          {'key': 'app.view', 'description': 'See apps', 'scope': 'project'},
        ],
      });
      expect(g.permissions.single.projectScope, isTrue);
    });
  });

  group('builds and registries', () {
    test('BuildInfo', () {
      final b = BuildInfo.fromJson({
        'id': 9,
        'number': 3,
        'trigger': 'webhook',
        'branch': 'main',
        'commit_sha': '0123456789abcdef0123456789abcdef01234567',
        'commit_message': 'feat: x',
        'status': 'running',
        'image': 'registry.nectar.local/prj-shop/api:build-3',
        'deploy': true,
        'release': 7,
      });
      expect(b.running, isTrue);
      expect(b.shortSha, '0123456');
      expect(b.release, 7);
    });

    test('AppSourceInfo and RegistrySettings', () {
      final s = AppSourceInfo.fromJson({
        'repo_url': 'https://github.com/acme/api.git',
        'branch': 'main',
        'has_token': true,
        'auto_deploy': true,
        'webhook_url': 'https://panel/api/hooks/git/3',
        'webhook_secret': 'abc',
        'build_cpu_millicores': 2000,
      });
      expect(s.hasToken, isTrue);
      expect(s.buildCpu, 2000);
      final r = RegistrySettings.fromJson({
        'logins': [
          {'id': 1, 'host': 'ghcr.io', 'username': 'me'},
        ],
        'builtin': {'enabled': true, 'host': 'registry.nectar.local'},
      });
      expect(r.logins.single.host, 'ghcr.io');
      expect(r.builtinEnabled, isTrue);
      final n = NodeInfo.fromJson({
        'name': 'w1',
        'unschedulable': true,
        'edge': false,
        'roles': [],
      });
      expect(n.unschedulable, isTrue);
    });
  });
}
