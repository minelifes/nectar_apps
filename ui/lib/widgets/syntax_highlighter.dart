import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/conf.dart';
import 'package:nectar_apps_ui/widgets/highlights/env.dart';
import 'package:nectar_apps_ui/widgets/highlights/html.dart';
import 'package:nectar_apps_ui/widgets/highlights/ini.dart';
import 'package:nectar_apps_ui/widgets/highlights/json.dart';
import 'package:nectar_apps_ui/widgets/highlights/php.dart';
import 'package:nectar_apps_ui/widgets/highlights/py.dart';
import 'package:nectar_apps_ui/widgets/highlights/toml.dart';
import 'package:nectar_apps_ui/widgets/highlights/ts.js.dart';
import 'package:nectar_apps_ui/widgets/highlights/xml.dart';
import 'package:nectar_apps_ui/widgets/highlights/yaml.dart';

import '../theme/tokens.dart';
import 'highlights/css.dart';
import 'highlights/sh.dart';
import 'highlights/syntax_rule.dart';

/// A [TextEditingController] that supports syntax highlighting by overriding
/// [buildTextSpan].
///
/// Use [rules] for custom rule-based highlighting, or [fileType] to reuse the
/// built-in [SyntaxHighlighter] logic.
///
/// ```dart
/// final controller = SyntaxHighlightController(
///   text: 'final message = "Hello world";',
///   fileType: 'dart',
/// );
///
/// TextField(
///   controller: controller,
///   style: const TextStyle(fontFamily: 'monospace', fontSize: 14),
/// )
/// ```
class SyntaxHighlightController extends TextEditingController {
  SyntaxHighlightController({
    super.text,
    required this.fileType,
    this.brightness = Brightness.dark,
  });

  final String fileType;
  final Brightness? brightness;

  List<SyntaxRule> getRules() {
    switch (fileType) {
      case 'conf':
        return confSyntaxRules;
      case 'env':
        return envSyntaxRules;
      case 'ini':
        return iniSyntaxRules;
      case 'json':
        return jsonSyntaxRules;
      case 'html':
        return htmlSyntaxRules;
      case 'css' || 'scss':
        return cssSyntaxRules;
      case 'sh':
        return shellSyntaxRules;
      case 'js' || 'ts':
        return jsTsSyntaxRules;
      case 'php':
        return phpSyntaxRules;
      case 'py':
        return pythonSyntaxRules;
      case 'toml':
        return tomlSyntaxRules;
      case 'xml':
        return xmlSyntaxRules;
      case 'yml' || 'yaml':
        return yamlSyntaxRules;
      default:
        return [];
    }
  }

  @override
  TextSpan buildTextSpan({
    required BuildContext context,
    TextStyle? style,
    required bool withComposing,
  }) {
    final source = value.text;

    final rules = getRules();

    if (source.isEmpty) {
      return TextSpan(style: style, text: source);
    }

    final spans = <TextSpan>[];
    var position = 0;

    while (position < source.length) {
      Match? selectedMatch;
      SyntaxRule? selectedRule;

      for (final rule in rules) {
        final match = rule.pattern.matchAsPrefix(source, position);

        if (match != null &&
            (selectedMatch == null || match.end > selectedMatch.end)) {
          selectedMatch = match;
          selectedRule = rule;
        }
      }

      if (selectedMatch == null) {
        spans.add(TextSpan(text: source[position], style: style));
        position++;
        continue;
      }

      spans.add(
        TextSpan(
          text: selectedMatch.group(0),
          style: style?.merge(selectedRule!.style),
        ),
      );

      position = selectedMatch.end;
    }

    return TextSpan(style: style, children: spans);
  }
}

/// Default monospace style for code input fields.
TextStyle _defaultStyle(BuildContext context, Brightness brightness) =>
    TextStyle(
      fontFamily: NectarTokens.fontMono,
      fontSize: 14,
      height: 1.45,
      color: brightness == Brightness.dark
          ? NectarTokens.darkText
          : NectarTokens.lightText,
    );

/// A syntax-highlighted code input field.
///
/// Uses a [TextField] with a [SyntaxHighlightController] that overrides
/// [buildTextSpan] to render syntax-highlighted text natively. No overlay
/// painter needed.
///
/// Usage:
/// ```dart
/// SyntaxHighlightedTextField(
///   controller: myController,
///   fileType: 'sh',
///   decoration: InputDecoration(labelText: 'Command'),
/// )
/// ```
class SyntaxHighlightedTextField extends StatefulWidget {
  const SyntaxHighlightedTextField({
    super.key,
    required this.controller,
    required this.fileType,
    this.decoration,
    this.brightness,
    this.style,
    this.maxLines,
    this.minLines,
    this.onChanged,
    this.autofocus = false,
    this.enabled = true,
  });

  final TextEditingController controller;
  final String fileType;
  final InputDecoration? decoration;
  final Brightness? brightness;
  final TextStyle? style;
  final int? maxLines;
  final int? minLines;
  final void Function(String)? onChanged;
  final bool autofocus;
  final bool enabled;

  @override
  State<SyntaxHighlightedTextField> createState() =>
      _SyntaxHighlightedTextFieldState();
}

class _SyntaxHighlightedTextFieldState
    extends State<SyntaxHighlightedTextField> {
  late final SyntaxHighlightController _highlightController;

  @override
  void initState() {
    super.initState();
    _highlightController = SyntaxHighlightController(
      text: widget.controller.text,
      fileType: widget.fileType,
      brightness: widget.brightness,
    );
    _syncFromWidget();
    _highlightController.addListener(_onHighlightChange);
  }

  @override
  void didUpdateWidget(covariant SyntaxHighlightedTextField oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.controller != oldWidget.controller) {
      _highlightController.removeListener(_onHighlightChange);
      _syncFromWidget();
      _highlightController.addListener(_onHighlightChange);
    }
  }

  @override
  void dispose() {
    _highlightController.removeListener(_onHighlightChange);
    _highlightController.dispose();
    super.dispose();
  }

  void _syncFromWidget() {
    if (_highlightController.text != widget.controller.text) {
      _highlightController.text = widget.controller.text;
    }
    _highlightController.selection = widget.controller.selection;
  }

  void _onHighlightChange() {
    widget.controller.text = _highlightController.text;
    widget.controller.selection = _highlightController.selection;
    widget.onChanged?.call(_highlightController.text);
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final effectiveStyle =
        widget.style ??
        _defaultStyle(context, widget.brightness ?? theme.brightness);

    return TextField(
      enabled: widget.enabled,
      controller: _highlightController,
      style: effectiveStyle,
      decoration:
          widget.decoration ??
          const InputDecoration(
            contentPadding: EdgeInsets.all(12),
            border: InputBorder.none,
          ),
      maxLines: widget.maxLines,
      minLines: widget.minLines,
      autocorrect: false,
      keyboardType: TextInputType.multiline,
      textInputAction: TextInputAction.newline,
      autofocus: widget.autofocus,
    );
  }
}
