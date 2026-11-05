import 'package:flutter/material.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final jsTsSyntaxRules = <SyntaxRule>[
  // 1. Comments
  SyntaxRule(
    RegExp(r'//[^\n]*|/\*[\s\S]*?\*/'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // 2. Template literals
  SyntaxRule(
    RegExp(r'`(?:\\[\s\S]|[^`])*`'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 3. Strings
  SyntaxRule(
    RegExp(r'''"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 4. Decorators
  SyntaxRule(
    RegExp(r'@[a-zA-Z_$][\w$]*'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // 5. Keywords
  SyntaxRule(
    RegExp(
      r'\b(as|async|await|break|case|catch|class|const|continue|'
      r'debugger|default|delete|do|else|export|extends|finally|'
      r'for|from|function|get|if|implements|import|in|instanceof|'
      r'let|new|of|return|set|static|super|switch|this|throw|'
      r'try|typeof|var|void|while|with|yield|'
      r'abstract|declare|enum|infer|interface|is|keyof|'
      r'module|namespace|never|override|private|protected|'
      r'public|readonly|satisfies|type|unique|unknown|'
      r'any|boolean|number|string|symbol|object|bigint)\b',
    ),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // 6. Boolean and null literals
  SyntaxRule(
    RegExp(r'\b(true|false|null|undefined|NaN|Infinity)\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 7. Function names
  SyntaxRule(
    RegExp(r'\b[a-zA-Z_$][\w$]*(?=\s*(?:<[^>]*>)?\s*\()'),
    const TextStyle(color: Color(0xFF61AFEF)),
  ),

  // 8. Class and interface names
  SyntaxRule(
    RegExp(
      r'\b(?:class|interface|enum|type|extends|implements)\s+'
      r'[A-Z][a-zA-Z0-9_$]*',
    ),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // 9. Constants (UPPER_CASE)
  SyntaxRule(
    RegExp(r'\b[A-Z][A-Z0-9_]{1,}\b'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // 10. Built-in objects
  SyntaxRule(
    RegExp(
      r'\b(Array|Object|String|Number|Boolean|Date|Math|JSON|'
      r'Map|Set|WeakMap|WeakSet|Promise|Symbol|RegExp|'
      r'Error|TypeError|RangeError|BigInt|Reflect|Proxy|'
      r'Intl|URL|URLSearchParams|console|globalThis)\b',
    ),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // 11. Numbers
  SyntaxRule(
    RegExp(
      r'\b(?:0[xX][0-9a-fA-F]+|0[bB][01]+|0[oO][0-7]+|'
      r'\d+(?:\.\d+)?(?:[eE][+-]?\d+)?n?)\b',
    ),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 12. Variables with $
  SyntaxRule(
    RegExp(r'\$[a-zA-Z_$][\w$]*'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // 13. Operators
  SyntaxRule(
    RegExp(
      r'===|!==|=>|\?\?=|\?\?|\?\.|'
      r'==|!=|<=|>=|&&|\|\||\+\+|--|'
      r'\+=|-=|\*=|/=|%=|\*\*=?|<<=?|>>=?|'
      r'&=|\|=|\^=|[+\-*/%<>=!&|^~?:]',
    ),
    const TextStyle(color: Color(0xFFE06C75)),
  ),

  // 14. Punctuation
  SyntaxRule(
    RegExp(r'[{}()\[\].,;]'),
    const TextStyle(color: Color(0xFFABB2BF)),
  ),
];
