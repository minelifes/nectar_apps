import 'dart:async';

import 'package:dio/dio.dart';

import 'token_store.dart';

/// An error returned by the panel API (`{"error": "..."}`) or a network failure (status == null).
class ApiException implements Exception {
  const ApiException(this.status, this.message);

  final int? status;
  final String message;

  bool get isUnauthorized => status == 401;
  bool get isForbidden => status == 403;
  bool get isNetwork => status == null;

  factory ApiException.from(DioException e) {
    final res = e.response;
    if (res == null) {
      return switch (e.type) {
        DioExceptionType.connectionTimeout ||
        DioExceptionType.receiveTimeout ||
        DioExceptionType.sendTimeout =>
          const ApiException(null, 'The server did not answer in time.'),
        DioExceptionType.badCertificate => const ApiException(null, 'The server certificate is not trusted.'),
        DioExceptionType.cancel => const ApiException(null, 'Cancelled.'),
        _ => const ApiException(null, 'Cannot reach the server. Check the address and your connection.'),
      };
    }
    final data = res.data;
    var msg = '';
    if (data is Map && data['error'] is String) msg = data['error'] as String;
    if (msg.isEmpty) {
      msg = switch (res.statusCode) {
        401 => 'Please sign in again.',
        403 => 'You do not have permission to do this.',
        404 => 'Not found.',
        429 => 'Too many attempts. Try again in a minute.',
        502 || 503 || 504 => 'The server is temporarily unavailable.',
        _ => 'Request failed (${res.statusCode}).',
      };
    }
    return ApiException(res.statusCode, msg);
  }

  @override
  String toString() => message;
}

/// Human-readable text for any error shown in the UI.
String describeError(Object error) {
  if (error is ApiException) return error.message;
  if (error is DioException) return ApiException.from(error).message;
  return error.toString();
}

/// HTTP client for the panel API.
///
/// Web builds served by the panel use the session cookie (same origin). Native apps and the web dev
/// server (`--dart-define=NECTAR_API=…`) use bearer tokens: a 15-minute access token, renewed with the
/// refresh token on the first 401.
class Api {
  Api({required this.baseUrl, required this.tokens, required this.useTokens, this.onSessionExpired}) {
    _dio = Dio(
      BaseOptions(
        baseUrl: '$baseUrl/api',
        connectTimeout: const Duration(seconds: 10),
        receiveTimeout: const Duration(seconds: 60),
        contentType: Headers.jsonContentType,
        responseType: ResponseType.json,
      ),
    );
    _dio.interceptors.add(InterceptorsWrapper(onRequest: _onRequest, onError: _onError));
  }

  final String baseUrl;
  final TokenStore tokens;
  final bool useTokens;

  /// Called when the session is gone (401 that a refresh could not fix).
  void Function()? onSessionExpired;

  late final Dio _dio;
  Completer<void>? _refreshing;

  /// Marks a request that must not carry or renew credentials (login, setup, refresh).
  static final Options public = Options(extra: {'public': true});

  void _onRequest(RequestOptions o, RequestInterceptorHandler h) {
    final access = tokens.access;
    if (useTokens && access != null && o.extra['public'] != true) {
      o.headers['Authorization'] = 'Bearer $access';
    }
    h.next(o);
  }

  Future<void> _onError(DioException e, ErrorInterceptorHandler h) async {
    final req = e.requestOptions;
    final public = req.extra['public'] == true;
    if (e.response?.statusCode == 401 && !public) {
      if (useTokens && tokens.refresh != null && req.extra['retried'] != true) {
        try {
          await _refresh();
          req.extra['retried'] = true;
          req.headers['Authorization'] = 'Bearer ${tokens.access}';
          h.resolve(await _dio.fetch<dynamic>(req));
          return;
        } on Object {
          // fall through: the session is gone
        }
      }
      onSessionExpired?.call();
    }
    h.next(e);
  }

  /// One refresh at a time; parallel 401s wait for it.
  Future<void> _refresh() async {
    final running = _refreshing;
    if (running != null) return running.future;
    final done = Completer<void>();
    done.future.ignore(); // only parallel callers listen; the first one gets the error directly
    _refreshing = done;
    try {
      final res = await _dio.post<dynamic>('/auth/refresh', data: {'refresh_token': tokens.refresh}, options: public);
      final data = res.data as Map<String, dynamic>;
      await tokens.save(data['access_token'] as String?, (data['refresh_token'] as String?) ?? tokens.refresh);
      done.complete();
    } catch (e) {
      await tokens.clear();
      done.completeError(e);
      rethrow;
    } finally {
      _refreshing = null;
    }
  }

  /// WebSocket URL for an API path. Token sessions pass the access token as a query parameter (browsers
  /// cannot set headers on WebSockets); the web build served by the panel uses its cookie.
  Uri wsUri(String path, [Map<String, String> query = const {}]) {
    final base = Uri.parse(baseUrl);
    final access = tokens.access;
    final q = {...query, if (useTokens && access != null) 'access_token': access};
    return base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '${base.path}/api$path',
      queryParameters: q.isEmpty ? null : q,
    );
  }

  Future<dynamic> get(String path, {Map<String, dynamic>? query, Options? options}) =>
      _send(() => _dio.get<dynamic>(path, queryParameters: query, options: options));

  Future<dynamic> post(String path, {Object? data, Options? options}) =>
      _send(() => _dio.post<dynamic>(path, data: data, options: options));

  Future<dynamic> put(String path, {Object? data}) => _send(() => _dio.put<dynamic>(path, data: data));

  Future<dynamic> patch(String path, {Object? data}) => _send(() => _dio.patch<dynamic>(path, data: data));

  Future<dynamic> delete(String path) => _send(() => _dio.delete<dynamic>(path));

  /// Streams a file as the raw request body (artifact upload) and reports progress.
  Future<dynamic> upload(
    String path, {
    required Stream<List<int>> data,
    required int length,
    Map<String, dynamic>? query,
    void Function(int sent, int total)? onProgress,
    CancelToken? cancel,
  }) =>
      _send(
        () => _dio.post<dynamic>(
          path,
          data: data,
          queryParameters: query,
          cancelToken: cancel,
          onSendProgress: onProgress,
          options: Options(
            contentType: 'application/octet-stream',
            headers: {Headers.contentLengthHeader: length},
            // no send timeout (none in BaseOptions): a slow uplink must not fail a big JAR;
            // the server answers after inspecting the file
            receiveTimeout: const Duration(minutes: 5),
          ),
        ),
      );

  Future<dynamic> _send(Future<Response<dynamic>> Function() call) async {
    try {
      return (await call()).data;
    } on DioException catch (e) {
      throw ApiException.from(e);
    }
  }
}
