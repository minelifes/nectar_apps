import 'package:flutter/material.dart';

import '../theme/tokens.dart';

// ---------------------------------------------------------------------------
// Syntax highlighting colors (dark-theme optimized, work in light mode too)
// ---------------------------------------------------------------------------

const _darkGreen = Color(0xFF40C890);
const _yellow = Color(0xFFF5D76E);
const _blue = Color(0xFF6CB4EE);
const _purple = Color(0xFFC792EA);
const _red = Color(0xFFFF6B6B);
const _cyan = Color(0xFF89D0F8);
const _orange = Color(0xFFF5A524);

/// A small helper used by [SyntaxHighlighter].
class _StringPart {
  final String text;
  final bool isString;
  _StringPart({required this.text, required this.isString});
}

/// Lightweight syntax highlighter for shell, YAML, JSON, config and markup
/// files.  Call [highlight] directly for [TextSpan] lists, or use the
/// [SyntaxView] widget to render them in a [RichText].
///
/// Supports these file types (case-insensitive):
///   - Shell: sh, bash, zsh
///   - YAML:  yaml, yml
///   - JSON:  json
///   - Config: conf, cfg, ini, properties
///   - Markup: html, xml
class SyntaxHighlighter {
  SyntaxHighlighter._(); // non-instantiable

  /// Highlight [text] according to [fileType] (extension, case-insensitive).
  /// [brightness] controls which text colour is used for the base style.
  static List<TextSpan> highlight(
    String text,
    String fileType, {
    Brightness brightness = Brightness.dark,
  }) {
    final baseStyle = TextStyle(
      fontFamily: NectarTokens.fontMono,
      fontSize: 13,
      height: 1.4,
      color: brightness == Brightness.dark
          ? NectarTokens.darkText
          : NectarTokens.lightText,
    );

    final spans = <TextSpan>[];
    final lines = text.split('\n');

    for (var i = 0; i < lines.length; i++) {
      final line = lines[i];
      final lineSpans = _highlightLine(line, fileType, brightness);
      for (final span in lineSpans) {
        spans.add(TextSpan(children: [span]));
      }
      if (i < lines.length - 1) {
        spans.add(TextSpan(text: '\n', style: baseStyle));
      }
    }

    return [TextSpan(style: baseStyle, children: spans)];
  }

  // ---- per-line dispatch --------------------------------------------------

  static List<TextSpan> _highlightLine(
    String line,
    String fileType,
    Brightness brightness,
  ) {
    final baseStyle = TextStyle(
      fontFamily: NectarTokens.fontMono,
      fontSize: 13,
      color: brightness == Brightness.dark
          ? NectarTokens.darkText
          : NectarTokens.lightText,
    );

    final ext = fileType.toLowerCase();

    switch (ext) {
      case 'sh':
      case 'bash':
      case 'zsh':
        return _highlightShell(line, baseStyle);
      case 'yaml':
      case 'yml':
        return _highlightYaml(line, baseStyle);
      case 'json':
        return _highlightJson(line, baseStyle);
      case 'conf':
      case 'cfg':
      case 'ini':
      case 'properties':
        return _highlightConfig(line, baseStyle);
      case 'html':
      case 'xml':
        return _highlightMarkup(line, baseStyle);
      default:
        return [TextSpan(text: line, style: baseStyle)];
    }
  }

  // ---- individual highlighters --------------------------------------------

  static List<TextSpan> _highlightShell(String line, TextStyle base) {
    final spans = <TextSpan>[];
    // ignore: prefer_final_locals — reassigned in the loop below
    var remaining = line;

    // Comments
    final commentIdx = remaining.indexOf('#');
    if (commentIdx >= 0) {
      spans.add(
        TextSpan(text: remaining.substring(0, commentIdx), style: base),
      );
      spans.add(
        TextSpan(
          text: remaining.substring(commentIdx),
          style: base.copyWith(color: _darkGreen, fontStyle: FontStyle.italic),
        ),
      );
      return [TextSpan(children: spans)];
    }

    // Strings
    final stringParts = _splitStrings(remaining);
    for (final part in stringParts) {
      if (part.isString) {
        spans.add(
          TextSpan(
            text: part.text,
            style: base.copyWith(color: _yellow),
          ),
        );
      } else {
        // Keywords
        if (_matchesKeyword(part.text, [
          'if',
          'then',
          'else',
          'fi',
          'elif',
          'for',
          'while',
          'do',
          'done',
          'case',
          'esac',
          'in',
        ])) {
          spans.add(
            TextSpan(
              text: part.text,
              style: base.copyWith(
                color: NectarTokens.amber,
                fontWeight: FontWeight.bold,
              ),
            ),
          );
        } else if (_matchesCommand(part.text, [
          'chmod',
          'chown',
          'mosquitto_passwd',
          'printf',
          'exec',
          'set',
          'export',
          'source',
          'cd',
          'mkdir',
          'touch',
          'cp',
          'mv',
          'rm',
          'chattr',
          'ls',
          'cat',
          'echo',
          'grep',
          'sed',
          'awk',
          'find',
          'docker',
          'kubectl',
          'apt',
          'pip',
          'npm',
        ])) {
          spans.add(
            TextSpan(
              text: part.text,
              style: base.copyWith(color: _cyan),
            ),
          );
        } else if (_matchesVar(part.text)) {
          spans.add(
            TextSpan(
              text: part.text,
              style: base.copyWith(color: _blue),
            ),
          );
        } else if (_matchesPath(part.text)) {
          spans.add(
            TextSpan(
              text: part.text,
              style: base.copyWith(color: _purple),
            ),
          );
        } else {
          spans.add(TextSpan(text: part.text, style: base));
        }
      }
    }

    return [TextSpan(children: spans)];
  }

  static List<TextSpan> _highlightYaml(String line, TextStyle base) {
    final spans = <TextSpan>[];

    // Comments
    final commentIdx = line.indexOf('#');
    if (commentIdx >= 0) {
      spans.add(TextSpan(text: line.substring(0, commentIdx), style: base));
      spans.add(
        TextSpan(
          text: line.substring(commentIdx),
          style: base.copyWith(color: _darkGreen, fontStyle: FontStyle.italic),
        ),
      );
      return [TextSpan(children: spans)];
    }

    // Keys
    final keyMatch = RegExp(r'^(\s*)([\w_-]+)(:)(.*)').firstMatch(line);
    if (keyMatch != null) {
      spans.add(TextSpan(text: keyMatch.group(1), style: base));
      spans.add(
        TextSpan(
          text: keyMatch.group(2),
          style: base.copyWith(color: _blue),
        ),
      );
      spans.add(TextSpan(text: keyMatch.group(3), style: base));
      final value = keyMatch.group(4) ?? '';
      if (value.isNotEmpty) {
        if (value.startsWith("'") || value.startsWith('"')) {
          spans.add(
            TextSpan(
              text: value,
              style: base.copyWith(color: _yellow),
            ),
          );
        } else if (value == 'true' || value == 'false' || value == 'null') {
          spans.add(
            TextSpan(
              text: value,
              style: base.copyWith(color: _orange),
            ),
          );
        } else {
          spans.add(TextSpan(text: value, style: base));
        }
      }
      return [TextSpan(children: spans)];
    }

    return [TextSpan(text: line, style: base)];
  }

  static List<TextSpan> _highlightJson(String line, TextStyle base) {
    final spans = <TextSpan>[];

    // Keys
    final keyMatch = RegExp(r'("[^"]+\")\s*(:)').firstMatch(line);
    if (keyMatch != null) {
      spans.add(
        TextSpan(
          text: keyMatch.group(1),
          style: base.copyWith(color: _blue),
        ),
      );
      spans.add(TextSpan(text: keyMatch.group(2), style: base));
      final rest = line.substring(keyMatch.end);
      if (rest.startsWith('"')) {
        final strEnd = rest.indexOf('"', 1);
        if (strEnd > 0) {
          spans.add(
            TextSpan(
              text: rest.substring(0, strEnd + 1),
              style: base.copyWith(color: _yellow),
            ),
          );
          if (strEnd + 1 < rest.length) {
            spans.add(TextSpan(text: rest.substring(strEnd + 1), style: base));
          }
        } else {
          spans.add(TextSpan(text: rest, style: base));
        }
      } else {
        spans.add(TextSpan(text: rest, style: base));
      }
      return [TextSpan(children: spans)];
    }

    return [TextSpan(text: line, style: base)];
  }

  static List<TextSpan> _highlightConfig(String line, TextStyle base) {
    final spans = <TextSpan>[];

    if (line.trim().isEmpty || line.trim().startsWith('#')) {
      final isComment = line.trim().startsWith('#');
      return [
        TextSpan(
          text: line,
          style: base.copyWith(
            color: isComment ? _darkGreen : null,
            fontStyle: isComment ? FontStyle.italic : null,
          ),
        ),
      ];
    }

    final eqIdx = line.indexOf('=');
    if (eqIdx > 0) {
      final key = line.substring(0, eqIdx).trim();
      final value = line.substring(eqIdx + 1);
      spans.add(
        TextSpan(
          text: key,
          style: base.copyWith(color: _blue),
        ),
      );
      spans.add(TextSpan(text: '=', style: base));
      if (value.startsWith("'") || value.startsWith('"')) {
        spans.add(
          TextSpan(
            text: value,
            style: base.copyWith(color: _yellow),
          ),
        );
      } else {
        spans.add(TextSpan(text: value, style: base));
      }
      return [TextSpan(children: spans)];
    }

    return [TextSpan(text: line, style: base)];
  }

  static List<TextSpan> _highlightMarkup(String line, TextStyle base) {
    final spans = <TextSpan>[];
    var remaining = line;

    while (remaining.isNotEmpty) {
      final tagMatch = RegExp(r'(<[!/]?[a-zA-Z][^>]*>)').firstMatch(remaining);
      if (tagMatch != null && tagMatch.start == 0) {
        spans.add(
          TextSpan(
            text: tagMatch.group(1)!,
            style: base.copyWith(color: _red),
          ),
        );
        remaining = remaining.substring(tagMatch.end);
      } else {
        final attrMatch = RegExp(
          r'([a-zA-Z-]+)(=)(&quot;|&#39;|"|""|&#39;|"|""|&#39;|"|""|\x27)([^&]*)(\3)',
        ).firstMatch(remaining);
        if (attrMatch != null) {
          spans.add(
            TextSpan(
              text: attrMatch.group(1),
              style: base.copyWith(color: _blue),
            ),
          );
          spans.add(TextSpan(text: attrMatch.group(2), style: base));
          spans.add(
            TextSpan(
              text: attrMatch.group(4),
              style: base.copyWith(color: _yellow),
            ),
          );
          remaining = remaining.substring(attrMatch.end);
        } else {
          spans.add(TextSpan(text: remaining, style: base));
          remaining = '';
        }
      }
    }

    return [TextSpan(children: spans)];
  }

  // ---- helpers -------------------------------------------------------------

  static bool _matchesKeyword(String text, List<String> keywords) {
    final trimmed = text.trim();
    return keywords.any((k) => trimmed == k || trimmed.startsWith('$k '));
  }

  static bool _matchesCommand(String text, List<String> commands) {
    final trimmed = text.trim();
    return commands.any((c) => trimmed.startsWith('$c ') || trimmed == c);
  }

  static bool _matchesVar(String text) {
    return RegExp(r'^\$[A-Za-z_][A-Za-z0-9_]*$').hasMatch(text.trim()) ||
        RegExp(r'^\$\{[^}]+\}$').hasMatch(text.trim()) ||
        RegExp(r'^\$[0-9]$').hasMatch(text.trim());
  }

  static bool _matchesPath(String text) {
    return RegExp(r'^[/~][\w./-]*$').hasMatch(text.trim());
  }

  static List<_StringPart> _splitStrings(String text) {
    final parts = <_StringPart>[];
    var remaining = text;
    while (remaining.isNotEmpty) {
      final singleQuote = remaining.indexOf("'");
      final doubleQuote = remaining.indexOf('"');
      int quoteIdx;
      bool isDouble;

      if (singleQuote == -1 && doubleQuote == -1) {
        parts.add(_StringPart(text: remaining, isString: false));
        break;
      } else if (singleQuote == -1) {
        quoteIdx = doubleQuote;
        isDouble = true;
      } else if (doubleQuote == -1) {
        quoteIdx = singleQuote;
        isDouble = false;
      } else {
        quoteIdx = singleQuote < doubleQuote ? singleQuote : doubleQuote;
        isDouble = doubleQuote < singleQuote;
      }

      if (quoteIdx > 0) {
        parts.add(
          _StringPart(text: remaining.substring(0, quoteIdx), isString: false),
        );
      }

      final endQuote = remaining.indexOf(isDouble ? '"' : "'", quoteIdx + 1);
      if (endQuote > quoteIdx) {
        parts.add(
          _StringPart(
            text: remaining.substring(quoteIdx, endQuote + 1),
            isString: true,
          ),
        );
        remaining = remaining.substring(endQuote + 1);
      } else {
        parts.add(
          _StringPart(text: remaining.substring(quoteIdx), isString: true),
        );
        break;
      }
    }
    return parts;
  }
}

/// Renders syntax-highlighted text using [RichText].
///
/// Pass the [fileType] (file extension) and optionally [brightness] to
/// match the current theme.
class SyntaxView extends StatelessWidget {
  const SyntaxView({
    super.key,
    required this.text,
    required this.fileType,
    Brightness? brightness,
  }) : _brightness = brightness;

  final String text;
  final String fileType;
  final Brightness? _brightness;

  @override
  Widget build(BuildContext context) {
    final brightness = _brightness ?? Theme.of(context).brightness;
    final spans = SyntaxHighlighter.highlight(
      text,
      fileType,
      brightness: brightness,
    );
    return SelectableRichText(
      text: TextSpan(
        style: TextStyle(
          fontFamily: NectarTokens.fontMono,
          fontSize: 13,
          height: 1.4,
          color: brightness == Brightness.dark
              ? NectarTokens.darkText
              : NectarTokens.lightText,
        ),
        children: spans,
      ),
    );
  }
}

/// A [SelectableText.rich] that doesn't crash on null style.
class SelectableRichText extends StatelessWidget {
  const SelectableRichText({required this.text, super.key});
  final TextSpan text;

  @override
  Widget build(BuildContext context) {
    return SelectableText.rich(text, textAlign: TextAlign.left);
  }
}
