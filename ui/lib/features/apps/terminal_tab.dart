import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:xterm/xterm.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../theme/tokens.dart';

/// A shell inside a running container of the app (bash when the image has it, sh otherwise).
class TerminalTab extends ConsumerStatefulWidget {
  const TerminalTab({super.key, required this.app, this.height = 520});

  final App app;
  final double height;

  @override
  ConsumerState<TerminalTab> createState() => _TerminalTabState();
}

/// Writes decoded UTF-8 output into the terminal (multi-byte characters may span messages).
class _TermSink implements Sink<String> {
  _TermSink(this.terminal);

  final Terminal terminal;

  @override
  void add(String data) => terminal.write(data);

  @override
  void close() {}
}

class _TerminalTabState extends ConsumerState<TerminalTab> {
  final _terminal = Terminal(maxLines: 10000);
  final _controller = TerminalController();
  late ByteConversionSink _decoder = const Utf8Decoder(allowMalformed: true).startChunkedConversion(_TermSink(_terminal));
  WebSocketChannel? _ch;
  StreamSubscription<dynamic>? _sub;
  List<PodBrief> _pods = const [];
  String? _pod; // null = newest running pod
  String _state = 'idle'; // idle | connecting | open | closed
  String? _message;
  int _cols = 80, _rows = 24;

  @override
  void initState() {
    super.initState();
    _terminal.onOutput = (data) => _send({'t': 'i', 'd': data});
    _terminal.onResize = (w, h, pw, ph) {
      _cols = w;
      _rows = h;
      _send({'t': 'r', 'c': w, 'r': h});
    };
    _loadPods();
    _connect();
  }

  @override
  void dispose() {
    _sub?.cancel();
    _ch?.sink.close();
    _controller.dispose();
    super.dispose();
  }

  void _send(Map<String, dynamic> m) {
    if (_state == 'open') _ch?.sink.add(jsonEncode(m));
  }

  Future<void> _loadPods() async {
    try {
      final pods = listOf(await ref.read(apiProvider).get('/apps/${widget.app.id}/pods'), PodBrief.fromJson);
      if (mounted) setState(() => _pods = pods.where((p) => p.phase == 'Running').toList());
    } on Object {
      // the pod list is only a convenience; the server picks a pod anyway
    }
  }

  Future<void> _connect() async {
    await _sub?.cancel();
    await _ch?.sink.close();
    final api = ref.read(apiProvider);
    setState(() {
      _state = 'connecting';
      _message = null;
    });
    _decoder = const Utf8Decoder(allowMalformed: true).startChunkedConversion(_TermSink(_terminal));
    try {
      await api.get('/apps/${widget.app.id}/pods'); // renews an expired access token before it goes into the URL
      final ch = WebSocketChannel.connect(api.wsUri('/apps/${widget.app.id}/exec/ws', {if (_pod != null) 'pod': _pod!}));
      _ch = ch;
      await ch.ready;
      if (!mounted) {
        await ch.sink.close();
        return;
      }
      _sub = ch.stream.listen(_onMessage, onDone: _closed, onError: (Object e) => _closed(describeError(e)));
    } on Object catch (e) {
      _closed(describeError(e));
    }
  }

  void _onMessage(dynamic data) {
    if (data is! String) return;
    final Object? m;
    try {
      m = jsonDecode(data);
    } on FormatException {
      return;
    }
    if (m is! Map) return;
    switch (m['t']) {
      case 's':
        setState(() {
          _state = 'open';
          _message = 'Connected to ${m['pod']}';
        });
        _terminal.write('\x1b[2m— ${m['pod']} —\x1b[0m\r\n');
        _send({'t': 'r', 'c': _cols, 'r': _rows});
      case 'o':
        _decoder.add(base64Decode('${m['b']}'));
      case 'x':
        _closed('${m['msg'] ?? 'Session ended'}');
    }
  }

  void _closed([String? why]) {
    if (!mounted || _state == 'closed') return;
    setState(() {
      _state = 'closed';
      _message = why ?? 'Session ended';
    });
    _terminal.write('\r\n\x1b[2m— ${_message!} —\x1b[0m\r\n');
  }

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final open = _state == 'open';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Wrap(
          spacing: 8,
          runSpacing: 8,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            DropdownButton<String?>(
              value: _pods.any((p) => p.name == _pod) ? _pod : null,
              underline: const SizedBox.shrink(),
              hint: const Text('Newest pod'),
              items: [
                const DropdownMenuItem<String?>(value: null, child: Text('Newest pod')),
                for (final p in _pods) DropdownMenuItem<String?>(value: p.name, child: Text(p.name)),
              ],
              onChanged: (v) {
                setState(() => _pod = v);
                _connect();
              },
            ),
            if (!open)
              FilledButton.icon(
                onPressed: _state == 'connecting' ? null : _connect,
                icon: const Icon(Icons.terminal_rounded, size: 18),
                label: Text(_state == 'connecting' ? 'Connecting…' : 'Open a shell'),
              )
            else
              OutlinedButton.icon(
                onPressed: () => _ch?.sink.close(),
                icon: const Icon(Icons.close_rounded, size: 18),
                label: const Text('Disconnect'),
              ),
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(shape: BoxShape.circle, color: open ? c.running : c.stopped),
                ),
                const SizedBox(width: 6),
                Text(_message ?? '', style: TextStyle(color: c.muted, fontSize: 12.5)),
              ],
            ),
          ],
        ),
        const SizedBox(height: 10),
        Container(
          height: widget.height,
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: const Color(0xFF0A0B0D),
            borderRadius: BorderRadius.circular(NectarTokens.radius),
            border: Border.all(color: c.border),
          ),
          child: TerminalView(
            _terminal,
            controller: _controller,
            autofocus: true,
            backgroundOpacity: 0,
            textStyle: const TerminalStyle(fontSize: 13, fontFamily: NectarTokens.fontMono),
          ),
        ),
        const SizedBox(height: 8),
        Text(
          'Changes inside the container are lost on restart, except on volumes. Every session is recorded in the audit log.',
          style: TextStyle(color: c.muted, fontSize: 12),
        ),
      ],
    );
  }
}
