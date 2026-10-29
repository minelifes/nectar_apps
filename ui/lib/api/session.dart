import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/server_config.dart';
import 'client.dart';
import 'models.dart';
import 'token_store.dart';

enum SessionStatus {
  /// resolving the server and the current user
  loading,

  /// native app without a server address
  needsServer,

  /// the panel has no users yet: first-run setup
  needsSetup,
  signedOut,
  signedIn,

  /// the server could not be reached (see [SessionState.error])
  unreachable,
}

class SessionState {
  const SessionState({
    required this.status,
    this.serverUrl,
    this.canChangeServer = false,
    this.user,
    this.permissions = Permissions.none,
    this.version,
    this.error,
  });

  final SessionStatus status;
  final String? serverUrl;
  final bool canChangeServer;
  final User? user;
  final Permissions permissions;
  final VersionInfo? version;
  final String? error;

  bool can(String permission, {int? projectId}) => permissions.can(permission, projectId: projectId);

  String get serverHost => serverUrl == null ? '' : (Uri.tryParse(serverUrl!)?.authority ?? serverUrl!);
}

final sessionProvider = NotifierProvider<SessionController, SessionState>(SessionController.new);

/// The API client of the current session. Providers that load data watch this, so they reload when the
/// server or the signed-in user changes.
final apiProvider = Provider<Api>((ref) {
  ref.watch(sessionProvider.select((s) => (s.serverUrl, s.user?.id)));
  final api = ref.read(sessionProvider.notifier).api;
  if (api == null) throw const ApiException(null, 'Not connected to a server.');
  return api;
});

class SessionController extends Notifier<SessionState> {
  final _tokens = TokenStore();
  Api? api;
  ServerConfig? _config;

  @override
  SessionState build() {
    Future.microtask(bootstrap);
    return const SessionState(status: SessionStatus.loading);
  }

  /// Resolves the server, then whether setup is needed, then the current user.
  Future<void> bootstrap() async {
    final cfg = await ServerConfig.load();
    _config = cfg;
    final base = cfg.baseUrl;
    state = SessionState(status: SessionStatus.loading, serverUrl: base, canChangeServer: cfg.canChange);
    if (base == null) {
      state = SessionState(status: SessionStatus.needsServer, canChangeServer: cfg.canChange);
      return;
    }
    if (cfg.useTokens) await _tokens.load();
    final client = Api(baseUrl: base, tokens: _tokens, useTokens: cfg.useTokens, onSessionExpired: _expired);
    api = client;

    try {
      final version = VersionInfo.fromJson(await client.get('/version', options: Api.public));
      final setup = await client.get('/setup', options: Api.public);
      if (setup is Map && setup['required'] == true) {
        state = _base(SessionStatus.needsSetup, version: version);
        return;
      }
      if (cfg.useTokens && !_tokens.hasAny) {
        state = _base(SessionStatus.signedOut, version: version);
        return;
      }
      try {
        await _loadUser(version);
      } on ApiException catch (e) {
        if (!e.isUnauthorized) rethrow;
        state = _base(SessionStatus.signedOut, version: version);
      }
    } on ApiException catch (e) {
      state = _base(SessionStatus.unreachable, error: e.message);
    }
  }

  SessionState _base(SessionStatus status, {VersionInfo? version, String? error}) => SessionState(
        status: status,
        serverUrl: _config?.baseUrl,
        canChangeServer: _config?.canChange ?? false,
        version: version ?? state.version,
        error: error,
      );

  Future<void> _loadUser(VersionInfo? version) async {
    final client = api!;
    final results = await Future.wait([client.get('/me'), client.get('/me/permissions')]);
    state = SessionState(
      status: SessionStatus.signedIn,
      serverUrl: _config?.baseUrl,
      canChangeServer: _config?.canChange ?? false,
      user: User.fromJson(results[0]),
      permissions: Permissions.fromJson(results[1]),
      version: version ?? state.version,
    );
  }

  void _expired() {
    if (state.status != SessionStatus.signedIn) return;
    unawaited(_tokens.clear());
    state = _base(SessionStatus.signedOut, error: 'Your session has ended. Please sign in again.');
  }

  String get _client => (_config?.useTokens ?? false) ? 'mobile' : 'web';

  Future<void> _startSession(Object? response) async {
    final m = response is Map ? response : const {};
    if (_config?.useTokens ?? false) {
      await _tokens.save(m['access_token'] as String?, m['refresh_token'] as String?);
    }
    await _loadUser(null);
  }

  /// Signs in. Returns a token for [loginMfa] when the account uses two-factor authentication.
  Future<String?> login(String email, String password) async {
    final res = await api!.post(
      '/auth/login',
      data: {'email': email.trim(), 'password': password, 'client': _client},
      options: Api.public,
    );
    if (res is Map && res['mfa_required'] == true) return res['mfa_token'] as String?;
    await _startSession(res);
    return null;
  }

  /// Second step of a two-factor sign-in: an authenticator code or a recovery code.
  Future<void> loginMfa(String token, String code) async {
    final res = await api!.post('/auth/mfa', data: {'mfa_token': token, 'code': code.trim()}, options: Api.public);
    await _startSession(res);
  }

  /// Emails a password reset link (the server answers the same for unknown addresses).
  Future<void> forgotPassword(String email) async {
    await api!.post('/auth/forgot', data: {'email': email.trim()}, options: Api.public);
  }

  Future<void> setup({required String token, required String name, required String email, required String password}) async {
    final res = await api!.post(
      '/setup',
      data: {'token': token.trim(), 'name': name.trim(), 'email': email.trim(), 'password': password, 'client': _client},
      options: Api.public,
    );
    await _startSession(res);
  }

  Future<void> logout() async {
    try {
      await api?.post('/auth/logout');
    } on ApiException {
      // the session is gone either way
    }
    await _tokens.clear();
    state = _base(SessionStatus.signedOut);
  }

  /// Native apps: check that [input] is a Nectar panel, remember it and start over.
  Future<void> connect(String input) async {
    final url = ServerConfig.normalize(input);
    final probe = Api(baseUrl: url, tokens: TokenStore(), useTokens: true);
    final v = await probe.get('/version', options: Api.public);
    if (v is! Map || v['api_version'] == null) {
      throw const ApiException(null, 'This address does not look like a Nectar Apps panel.');
    }
    await _tokens.clear();
    await ServerConfig.save(url);
    await bootstrap();
  }

  /// Native apps: forget the server (and its tokens).
  Future<void> disconnect() async {
    await logout();
    await ServerConfig.save(null);
    api = null;
    await bootstrap();
  }
}
