// DO NOT EDIT. This is code generated via package:intl/generate_localized.dart
// This is a library that provides messages for a uk locale. All the
// messages from the main program should be duplicated here with the same
// function name.

// Ignore issues from commonly used lints in this file.
// ignore_for_file:unnecessary_brace_in_string_interps, unnecessary_new
// ignore_for_file:prefer_single_quotes,comment_references, directives_ordering
// ignore_for_file:annotate_overrides,prefer_generic_function_type_aliases
// ignore_for_file:unused_import, file_names, avoid_escaping_inner_quotes
// ignore_for_file:unnecessary_string_interpolations, unnecessary_string_escapes

import 'package:intl/intl.dart';
import 'package:intl/message_lookup_by_library.dart';

final messages = new MessageLookup();

typedef String MessageIfAbsent(String messageStr, List<dynamic> args);

class MessageLookup extends MessageLookupByLibrary {
  String get localeName => 'uk';

  static String m0(createdAt) => "Додано ${createdAt}";

  static String m1(label) => "Видалити ${label}?";

  static String m2(rName) => "Видалити роль ${rName}?";

  static String m3(rName) => "Лише ${rName}";

  static String m4(pName) => "Лише ${pName}";

  static String m5(tName) => "Відкликати ${tName}?";

  static String m6(minMemoryMb) => "Щонайменше ${minMemoryMb}";

  static String m7(addonName) => "Видалити ${addonName}?";

  static String m8(addonName) => "Пов\'язати ${addonName} із застосунком";

  static String m9(appName) =>
      "Пов\'язано. Розгорніть ${appName} знову, щоб застосувати.";

  static String m10(host, port) =>
      "Доступна лише застосункам того самого проєкту за адресою ${host}:${port}. Пов\'яжіть застосунок нижче, щоб він отримав ці параметри як змінні.";

  static String m11(vars) => "Усі застосунки отримують: ${vars}";

  static String m12(alertsLength) => "і ще ${alertsLength}";

  static String m13(chName) => "Видалити ${chName}?";

  static String m14(title) => "${title}: введіть число";

  static String m15(chName) => "Тестове повідомлення надіслано в ${chName}";

  static String m16(appName) =>
      "Збірку розпочато: після завершення вона розгорне ${appName}";

  static String m17(list) =>
      "Змінені ${list} збережено, але ще не застосовано.";

  static String m18(host) => "Скопійовано ${host}";

  static String m19(e) => "Не вдалося відкинути: ${e}";

  static String m20(appName) => "Видалити ${appName}?";

  static String m21(what) => "Видалити ${what}?";

  static String m22(slug) => "Видалити попередній перегляд ${slug}?";

  static String m23(vName) => "Видалити дані ${vName}?";

  static String m24(volume) => "Файли · ${volume}";

  static String m25(eName) =>
      "${eName} не є текстовим файлом — завантажте його натомість";

  static String m26(appName) =>
      "${appName} збережено як «не розгорнуто»; можна закрити майстер і продовжити пізніше зі сторінки застосунку.";

  static String m27(port, proto, nodePort) =>
      "Порт ${port}/${proto} відкрито на сервері як ${nodePort}";

  static String m28(days) => "${days} дн.";

  static String m29(parent, number) =>
      "Попередній перегляд ${parent} · pull request #${number}";

  static String m30(parent) =>
      "Перезбирається після нових комітів і видаляється, коли pull request закрито. Налаштування змінюйте в ${parent}.";

  static String m31(t) => "Просунути в ${t}…";

  static String m32(ready, replicas) => "${ready} / ${replicas} готові";

  static String m33(host) => "Прибрати ${host}?";

  static String m34(path) => "Прибрати ${path}?";

  static String m35(fName) => "Замінити ${fName}?";

  static String m36(eName) => "Збережено ${eName}";

  static String m37(index, stepsLength) => "Крок ${index} з ${stepsLength}";

  static String m38(label) =>
      "Цей файл більше підходить для ${label}, ніж вибрана версія.";

  static String m39(slug) => "Введіть ${slug} для підтвердження:";

  static String m40(size) => "${size} завантажено й перевірено";

  static String m41(round) => "Завантаження… ${round}%";

  static String m42(label) => "Використати ${label}";

  static String m43(vName) => "Том ${vName} видалено";

  static String m44(appName) =>
      "«${appName}» створено, але ще не розгорнуто. Залиште його, щоб завершити пізніше, або відкиньте.";

  static String m45(pName) => "Продовжити з ${pName}";

  static String m46(subject) => "Резервне копіювання ${subject} розпочато";

  static String m47(what) => "${what} Зміни після цього моменту буде втрачено.";

  static String m48(tName) => "Перевірка ${tName}…";

  static String m49(nextRun) => "Наступна: ${nextRun}";

  static String m50(subject, targetName) =>
      "Резервних копій ${subject} у ${targetName} більше не буде. Наявні знімки залишаються в репозиторії.";

  static String m51(tName) => "Прибрати ${tName}?";

  static String m52(shortId) => "Відновити ${shortId}?";

  static String m53(subject) => "Знімки · ${subject}";

  static String m54(count) =>
      "${Intl.plural(count, zero: 'Немає застосунків', one: '${count} застосунок', few: '${count} застосунки', many: '${count} застосунків', other: '${count} застосунку')}";

  static String m55(baseDomain, baseHost) =>
      "Запис *.${baseDomain} обслуговує типовий домен кожного застосунку. CNAME на ${baseHost} працює для піддоменів ваших власних доменів, щойно цей запис існує.";

  static String m56(ip) => "Заблокувати ${ip}";

  static String m57(domain) => "Видалити ${domain}?";

  static String m58(baseWildcard) => "Отримати ${baseWildcard}";

  static String m59(ms) => "${ms} мс";

  static String m60(pod) => "Перед останнім перезапуском · ${pod}";

  static String m61(linesLength) => "Скопійовано рядків: ${linesLength}";

  static String m62(visibleLength) => "Скопійовано рядків: ${visibleLength}";

  static String m63(replicas) =>
      "Сума по ${replicas} репліках; пунктир — скільки вони запитали разом.";

  static String m64(sampleImage) => "Образ: ${sampleImage} · 50m CPU · 64 МБ";

  static String m65(what) => "Вибрати ${what}";

  static String m66(slug) =>
      "Скопійовано — застосуйте командою: nectar apply -f nectar.yaml --project ${slug}";

  static String m67(appName) => "Копіювати ${appName}";

  static String m68(gName) => "Видалити групу ${gName}?";

  static String m69(slug) => "Середовище ${slug}";

  static String m70(what) => "Імпортувати ${what}";

  static String m71(key) =>
      "Неприпустима назва змінної «${key}»: використовуйте літери, цифри та _";

  static String m72(appName, target) => "Просунути ${appName} у ${target}?";

  static String m73(slug, slug2, slug3) =>
      "Активна збірка ${slug} (її образ або завантажений файл) розгортається в ${slug2}/${slug3}. Налаштування й змінні там залишаються як є.";

  static String m74(nodeName) => "Звільнити ${nodeName}?";

  static String m75(nodeName) => "Все одно звільнити ${nodeName}?";

  static String m76(historyName, range) =>
      "Історія · ${historyName} · останні ${range}";

  static String m77(keepArtifacts, auditDays, alertDays) =>
      "Зберігаються: останні ${keepArtifacts} завантажень на застосунок (і кожне, яке використовує активний реліз), ${auditDays} днів журналу аудиту, ${alertDays} днів вирішених сповіщень. Очищення щогодини.";

  static String m78(nodeName) => "Зробити ${nodeName} вузлом входу?";

  static String m79(nodeName) => "Видалити ${nodeName} з кластера?";

  static String m80(nodeName) =>
      "Припинити обслуговувати трафік на ${nodeName}?";

  static String m81(requestedBytes) =>
      "${requestedBytes} · використання видно, коли застосунок чи база даних працює";

  static String m82(ip, lastSeenAt, createdAt) =>
      "${ip} · активність ${lastSeenAt} · з ${createdAt}";

  static String m83(ip) =>
      "Дозволити лише ці IP-адреси чи мережі (по одній у рядку, напр. 203.0.113.7 або 203.0.113.0/24). Порожньо = усі. Ваша адреса: ${ip}. SSH-тунель до сервера працює завжди, тож помилковий список можна виправити.";

  static String m84(username, updatedAt) =>
      "${username} · змінено ${updatedAt}";

  static String m85(left) =>
      "Залишилося кодів відновлення: ${left}. Створіть нові.";

  static String m86(name) => "Видалити ключ доступу ${name}?";

  static String m87(when) => "використано ${when}";

  static String m88(pName) => "Прибрати ${pName}?";

  static String m89(host) => "Прибрати вхід для ${host}?";

  static String m90(amount) =>
      "Годинник сервера поспішає відносно цього пристрою на ${amount}: час у панелі показується неправильно, а коди двофакторної автентифікації й сертифікати можуть не працювати. На сервері виконайте: sudo timedatectl set-ntp true";

  static String m91(amount) =>
      "Годинник сервера відстає від цього пристрою на ${amount}: час у панелі показується неправильно, а коди двофакторної автентифікації й сертифікати можуть не працювати. На сервері виконайте: sudo timedatectl set-ntp true";

  static String m92(v) =>
      "Оновлення k3s до ${v} розпочато: версії вузлів тут оновлюватимуться в міру просування";

  static String m93(summary) => "Відкотити ${summary}?";

  static String m94(component) => "Оновити ${component}";

  static String m95(v) => "Оновити k3s до ${v}?";

  static String m96(summary) => "Оновлення · ${summary}";

  final messages = _notInlinedMessages(_notInlinedMessages);
  static Map<String, Function> _notInlinedMessages(_) => <String, Function>{
    "acceARoleIsASet": MessageLookupByLibrary.simpleMessage(
      "Роль — це набір дозволів. Користувачі отримують ролі всюди або для одного проєкту; API-токени можна обмежити роллю.",
    ),
    "acceATokenNeverHasMore": MessageLookupByLibrary.simpleMessage(
      "Токен ніколи не має більше прав, ніж ви. Для CI: роль Deployer і один проєкт.",
    ),
    "acceAddMember": MessageLookupByLibrary.simpleMessage("Додати учасника"),
    "acceAddRole": MessageLookupByLibrary.simpleMessage("Додати роль"),
    "acceAddUser": MessageLookupByLibrary.simpleMessage("Додати користувача"),
    "acceAdded": m0,
    "acceAllNone": MessageLookupByLibrary.simpleMessage("усі / жодного"),
    "acceAnyHttpClientWorksToo": MessageLookupByLibrary.simpleMessage(
      "Підійде й будь-який HTTP-клієнт: Authorization: Bearer nat_… до API (див. api/openapi.yaml).",
    ),
    "acceApiTokens": MessageLookupByLibrary.simpleMessage("API-токени"),
    "acceBuiltInRolesCannotBe": MessageLookupByLibrary.simpleMessage(
      "Вбудовані ролі змінювати не можна. Клонуйте роль, щоб створити власну версію.",
    ),
    "acceCannotSignInSessionsAnd": MessageLookupByLibrary.simpleMessage(
      "Не може входити; сеанси й токени перестають працювати",
    ),
    "acceChangingRolesNeedsRoleManage": MessageLookupByLibrary.simpleMessage(
      "Для зміни ролей потрібен дозвіл role.manage.",
    ),
    "acceClone": MessageLookupByLibrary.simpleMessage("Клонувати"),
    "acceCopyYourToken": MessageLookupByLibrary.simpleMessage(
      "Скопіюйте свій токен",
    ),
    "acceCreateOneForGithubActions": MessageLookupByLibrary.simpleMessage(
      "Створіть токен для GitHub Actions або nectar CLI. Для розгортання достатньо токена з роллю Deployer для одного проєкту.",
    ),
    "acceDelete": m1,
    "acceDeleteRole": m2,
    "acceDescription": MessageLookupByLibrary.simpleMessage("Опис"),
    "acceDisabled": MessageLookupByLibrary.simpleMessage("Вимкнено"),
    "acceEveryoneSTokens": MessageLookupByLibrary.simpleMessage(
      "Токени всіх користувачів",
    ),
    "acceEverythingYouMayDo": MessageLookupByLibrary.simpleMessage(
      "Усе, що можете ви",
    ),
    "acceEverywhere": MessageLookupByLibrary.simpleMessage("Усюди"),
    "acceExpires": MessageLookupByLibrary.simpleMessage("Спливає"),
    "acceForCiAndScriptsA": MessageLookupByLibrary.simpleMessage(
      "Для CI і скриптів. Токен діє від вашого імені, обмежений роллю й проєктом, якщо ви їх виберете.",
    ),
    "acceGithubActionsShop": MessageLookupByLibrary.simpleMessage(
      "GitHub Actions: shop",
    ),
    "acceGlobalOnlyPermissionsHaveNo": MessageLookupByLibrary.simpleMessage(
      "Дозволи «лише глобально» не діють, коли роль надано для одного проєкту.",
    ),
    "acceIn30Days": MessageLookupByLibrary.simpleMessage("через 30 днів"),
    "acceIn90Days": MessageLookupByLibrary.simpleMessage("через 90 днів"),
    "acceInAYear": MessageLookupByLibrary.simpleMessage("через рік"),
    "acceItIsShownOnlyNow": MessageLookupByLibrary.simpleMessage(
      "Він показується лише зараз. Збережіть його як секрет (напр. NECTAR_TOKEN у GitHub).",
    ),
    "acceMembers": MessageLookupByLibrary.simpleMessage("Учасники"),
    "acceNewApiToken": MessageLookupByLibrary.simpleMessage("Новий API-токен"),
    "acceNewRole": MessageLookupByLibrary.simpleMessage("Нова роль"),
    "acceNewToken": MessageLookupByLibrary.simpleMessage("Новий токен"),
    "acceNewUsersHaveNoRoles": MessageLookupByLibrary.simpleMessage(
      "Нові користувачі не мають ролей: призначте роль після створення.",
    ),
    "acceNoTokens": MessageLookupByLibrary.simpleMessage("Токенів немає."),
    "acceNoTokensYet": MessageLookupByLibrary.simpleMessage("Токенів ще немає"),
    "acceOnly": m3,
    "acceOnly2": m4,
    "acceOwnersCanDoEverythingRoles": MessageLookupByLibrary.simpleMessage(
      "Власники можуть усе; ролі цього не змінюють.",
    ),
    "accePassword10Characters": MessageLookupByLibrary.simpleMessage(
      "Пароль (від 10 символів)",
    ),
    "acceRemoveFromProject": MessageLookupByLibrary.simpleMessage(
      "Прибрати з проєкту",
    ),
    "acceReset2fa": MessageLookupByLibrary.simpleMessage("Скинути 2FA"),
    "acceRevoke": m5,
    "acceRevoke2": MessageLookupByLibrary.simpleMessage("Відкликати"),
    "acceRights": MessageLookupByLibrary.simpleMessage("Права"),
    "acceRole": MessageLookupByLibrary.simpleMessage("Роль"),
    "acceRoles": MessageLookupByLibrary.simpleMessage("Ролі"),
    "acceRolesStillAssignedToUsers": MessageLookupByLibrary.simpleMessage(
      "Ролі, які ще призначені користувачам, видалити не можна.",
    ),
    "acceScriptsAndPipelinesThatUse": MessageLookupByLibrary.simpleMessage(
      "Скрипти й конвеєри, що його використовують, одразу перестануть працювати.",
    ),
    "acceSendAnInvitation": MessageLookupByLibrary.simpleMessage(
      "Надіслати запрошення",
    ),
    "acceSendInvitationResetLink": MessageLookupByLibrary.simpleMessage(
      "Надіслати запрошення / посилання для скидання",
    ),
    "acceShareTheInvitationLink": MessageLookupByLibrary.simpleMessage(
      "Поділіться посиланням-запрошенням",
    ),
    "acceThePanelCannotSendEmails": MessageLookupByLibrary.simpleMessage(
      "Панель не може надсилати листи (Налаштування → Пошта). Надішліть це посилання самі; воно діє 7 днів.",
    ),
    "acceTheyCanNoLongerSign": MessageLookupByLibrary.simpleMessage(
      "Користувач більше не зможе входити; його API-токени перестануть працювати. Журнал аудиту зберігає його дії.",
    ),
    "acceTheyChooseTheirOwnPassword": MessageLookupByLibrary.simpleMessage(
      "Пароль користувач вибере сам",
    ),
    "acceUser": MessageLookupByLibrary.simpleMessage("Користувач"),
    "acceUserCreated": MessageLookupByLibrary.simpleMessage(
      "Користувача створено",
    ),
    "acceUsers": MessageLookupByLibrary.simpleMessage("Користувачі"),
    "acceUsingAToken": MessageLookupByLibrary.simpleMessage(
      "Як використовувати токен",
    ),
    "acceWhere": MessageLookupByLibrary.simpleMessage("Де"),
    "acceWhoCanSignInAnd": MessageLookupByLibrary.simpleMessage(
      "Хто може входити і що їм дозволено. Ролі діють всюди або в одному проєкті.",
    ),
    "acceYouCanOnlyGiveOr": MessageLookupByLibrary.simpleMessage(
      "Надавати чи забирати можна лише ролі, дозволи яких ви маєте самі.",
    ),
    "acceYouCanOnlyGiveRoles": MessageLookupByLibrary.simpleMessage(
      "Можна надавати лише ролі, дозволи яких ви маєте в цьому проєкті.",
    ),
    "acceYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Ваша роль не має дозволу user.manage.",
    ),
    "acceYourTokens": MessageLookupByLibrary.simpleMessage("Ваші токени"),
    "actionCancel": MessageLookupByLibrary.simpleMessage("Скасувати"),
    "actionClose": MessageLookupByLibrary.simpleMessage("Закрити"),
    "actionDelete": MessageLookupByLibrary.simpleMessage("Видалити"),
    "actionOk": MessageLookupByLibrary.simpleMessage("OK"),
    "actionRefresh": MessageLookupByLibrary.simpleMessage("Оновити"),
    "actionRetry": MessageLookupByLibrary.simpleMessage("Спробувати ще"),
    "actionSave": MessageLookupByLibrary.simpleMessage("Зберегти"),
    "addonAddADatabase": MessageLookupByLibrary.simpleMessage(
      "Додати базу даних",
    ),
    "addonAddDatabase": MessageLookupByLibrary.simpleMessage(
      "Додати базу даних",
    ),
    "addonApp": MessageLookupByLibrary.simpleMessage("Застосунок"),
    "addonAtLeast": m6,
    "addonCache": MessageLookupByLibrary.simpleMessage("CACHE_"),
    "addonConnection": MessageLookupByLibrary.simpleMessage("Підключення"),
    "addonDatabase": MessageLookupByLibrary.simpleMessage("База даних"),
    "addonDelete": m7,
    "addonDeleteDatabase": MessageLookupByLibrary.simpleMessage(
      "Видалити базу даних",
    ),
    "addonEveryAppOfTheProject": MessageLookupByLibrary.simpleMessage(
      "Усі застосунки проєкту вже пов\'язані",
    ),
    "addonForASecondDatabaseCache": MessageLookupByLibrary.simpleMessage(
      "Для другої бази даних: CACHE_ дає CACHE_DATABASE_URL",
    ),
    "addonLink": MessageLookupByLibrary.simpleMessage("Пов\'язати"),
    "addonLinkAnApp": MessageLookupByLibrary.simpleMessage(
      "Пов\'язати застосунок",
    ),
    "addonLinkOverridesShared": MessageLookupByLibrary.simpleMessage(
      "Застосунок, пов\'язаний тут, натомість використовує власний зв\'язок (і префікс).",
    ),
    "addonLinkToAnApp": m8,
    "addonLinkedApps": MessageLookupByLibrary.simpleMessage(
      "Пов\'язані застосунки",
    ),
    "addonLinkedRedeployToApply": m9,
    "addonLowercaseLettersDigitsDashes": MessageLookupByLibrary.simpleMessage(
      "Малі латинські літери, цифри, дефіси",
    ),
    "addonMemoryMib": MessageLookupByLibrary.simpleMessage("Пам\'ять (МіБ)"),
    "addonNameAlsoTheHostName": MessageLookupByLibrary.simpleMessage(
      "Назва (вона ж ім\'я хоста)",
    ),
    "addonNoAppUsesThisDatabase": MessageLookupByLibrary.simpleMessage(
      "Цю базу даних ще не використовує жоден застосунок.",
    ),
    "addonPostgresqlMysqlMariadbOrRedis": MessageLookupByLibrary.simpleMessage(
      "PostgreSQL, MySQL, MariaDB або Redis з постійним томом. Пов\'яжіть із застосунком, і він отримає DATABASE_URL (або REDIS_URL).",
    ),
    "addonReachableOnlyFromAppsIn": m10,
    "addonSavedTheDatabaseRestartsWith": MessageLookupByLibrary.simpleMessage(
      "Збережено; база даних перезапуститься з новими ресурсами",
    ),
    "addonShareHelp": MessageLookupByLibrary.simpleMessage(
      "Кожен застосунок проєкту отримує змінні підключення, зокрема ті, що ви додасте пізніше. Пов\'язувати не потрібно. Щоб застосувати, перерозгорніть застосунки.",
    ),
    "addonSharePrefix": MessageLookupByLibrary.simpleMessage(
      "Префікс спільних змінних",
    ),
    "addonShareWithProject": MessageLookupByLibrary.simpleMessage(
      "Спільна для всього проєкту",
    ),
    "addonSharedAs": m11,
    "addonSharedSaved": MessageLookupByLibrary.simpleMessage(
      "Збережено. Перерозгорніть застосунки, щоб застосувати.",
    ),
    "addonShowCredentials": MessageLookupByLibrary.simpleMessage(
      "Показати облікові дані",
    ),
    "addonStorageMib": MessageLookupByLibrary.simpleMessage("Сховище (МіБ)"),
    "addonTabEvents": MessageLookupByLibrary.simpleMessage("Події"),
    "addonTabLogs": MessageLookupByLibrary.simpleMessage("Логи"),
    "addonTabMetrics": MessageLookupByLibrary.simpleMessage("Метрики"),
    "addonTabOverview": MessageLookupByLibrary.simpleMessage("Огляд"),
    "addonTabTerminal": MessageLookupByLibrary.simpleMessage("Термінал"),
    "addonTheDatabaseAndAllIts": MessageLookupByLibrary.simpleMessage(
      "Базу даних і всі її дані буде видалено назавжди. Пов\'язані застосунки втратять параметри підключення з наступним розгортанням.",
    ),
    "addonUnlink": MessageLookupByLibrary.simpleMessage("Від\'єднати"),
    "addonVariablePrefixOptional": MessageLookupByLibrary.simpleMessage(
      "Префікс змінних (необов\'язково)",
    ),
    "alertAddChannel": MessageLookupByLibrary.simpleMessage("Додати канал"),
    "alertAlertRulesSaved": MessageLookupByLibrary.simpleMessage(
      "Правила сповіщень збережено",
    ),
    "alertAlertsWillNoLongerBe": MessageLookupByLibrary.simpleMessage(
      "Сповіщення більше не надсилатимуться туди.",
    ),
    "alertAllAlerts": MessageLookupByLibrary.simpleMessage("Усі сповіщення"),
    "alertAndMore": m12,
    "alertChangingRulesNeedsTheSettings": MessageLookupByLibrary.simpleMessage(
      "Для зміни правил потрібен дозвіл settings.manage.",
    ),
    "alertChannels": MessageLookupByLibrary.simpleMessage("Канали"),
    "alertDelete": m13,
    "alertEncryption": MessageLookupByLibrary.simpleMessage("Шифрування"),
    "alertEnterANumber": m14,
    "alertForAtLeast": MessageLookupByLibrary.simpleMessage(
      "Щонайменше протягом",
    ),
    "alertGetNotifiedWhenAppsGo": MessageLookupByLibrary.simpleMessage(
      "Отримуйте сповіщення, коли застосунки падають, серверам бракує ресурсів або спливають сертифікати.",
    ),
    "alertMarkAllRead": MessageLookupByLibrary.simpleMessage("Прочитати всі"),
    "alertMarkRead": MessageLookupByLibrary.simpleMessage(
      "Позначити прочитаним",
    ),
    "alertMarkUnread": MessageLookupByLibrary.simpleMessage(
      "Позначити непрочитаним",
    ),
    "alertNoChannelsYet": MessageLookupByLibrary.simpleMessage(
      "Каналів ще немає",
    ),
    "alertNoNotificationChannelYetAlerts": MessageLookupByLibrary.simpleMessage(
      "Каналу сповіщень ще немає: сповіщення видно лише тут. Додайте ntfy, Telegram, email або вебхук у розділі «Канали».",
    ),
    "alertNonePort25LocalRelays": MessageLookupByLibrary.simpleMessage(
      "Немає (порт 25, лише локальні ретранслятори)",
    ),
    "alertNotifications": MessageLookupByLibrary.simpleMessage("Сповіщення"),
    "alertNotificationsLinkToTheAffected": MessageLookupByLibrary.simpleMessage(
      "Сповіщення містять посилання на відповідний застосунок. Залиште порожнім, щоб без посилань.",
    ),
    "alertNtfyIsTheQuickestInstall": MessageLookupByLibrary.simpleMessage(
      "Найшвидше — ntfy: встановіть застосунок ntfy, підпишіться на тему, назву якої знаєте лише ви, і додайте її тут.",
    ),
    "alertPanelAddressForLinks": MessageLookupByLibrary.simpleMessage(
      "Адреса панелі для посилань",
    ),
    "alertReadLabel": MessageLookupByLibrary.simpleMessage("прочитано"),
    "alertRemindMeWhileAnAlert": MessageLookupByLibrary.simpleMessage(
      "Нагадувати, поки сповіщення активне",
    ),
    "alertSendATest": MessageLookupByLibrary.simpleMessage("Надіслати тест"),
    "alertSendWithThePanelS": MessageLookupByLibrary.simpleMessage(
      "Надсилати з налаштуваннями пошти панелі",
    ),
    "alertSettingsEmailTurnOffTo": MessageLookupByLibrary.simpleMessage(
      "Налаштування → Пошта; вимкніть, щоб використовувати інший SMTP-сервер",
    ),
    "alertStarttlsPort587": MessageLookupByLibrary.simpleMessage(
      "STARTTLS (порт 587)",
    ),
    "alertTestMessageSentTo": m15,
    "alertTlsPort465": MessageLookupByLibrary.simpleMessage("TLS (порт 465)"),
    "app0NoLimitTimeout60": MessageLookupByLibrary.simpleMessage(
      "0 = без обмеження (тайм-аут: 60 с). Понад ліміт клієнт отримує 429 (Too Many Requests); того, хто не зупиняється, блокують на певний час (Шлюз → Заблоковані IP). Одне завантаження сторінки може робити десятки запитів. Старіші шлюзи блокують одразу на 24 год і потребують щонайменше 10.",
    ),
    "app1Year": MessageLookupByLibrary.simpleMessage("1 рік"),
    "app2Years": MessageLookupByLibrary.simpleMessage("2 роки"),
    "app5MinutesTesting": MessageLookupByLibrary.simpleMessage(
      "5 хвилин (тестування)",
    ),
    "app6Months": MessageLookupByLibrary.simpleMessage("6 місяців"),
    "appAddDomain": MessageLookupByLibrary.simpleMessage("Додати домен"),
    "appAddFile": MessageLookupByLibrary.simpleMessage("Додати файл"),
    "appAddRedirectRule": MessageLookupByLibrary.simpleMessage(
      "Додати правило перенаправлення",
    ),
    "appAddVariable": MessageLookupByLibrary.simpleMessage("Додати змінну"),
    "appAddVariables": MessageLookupByLibrary.simpleMessage("Додати змінні"),
    "appAddVolume": MessageLookupByLibrary.simpleMessage("Додати том"),
    "appAllowCookiesCredentials": MessageLookupByLibrary.simpleMessage(
      "Дозволити cookies / облікові дані",
    ),
    "appAllowedIps": MessageLookupByLibrary.simpleMessage("Дозволені IP"),
    "appAllowedOrigins": MessageLookupByLibrary.simpleMessage(
      "Дозволені джерела (origins)",
    ),
    "appAlsoAvailableToTheApp": MessageLookupByLibrary.simpleMessage(
      "Також доступний застосунку як \$PORT",
    ),
    "appAlsoRedirectFromAndKeep": MessageLookupByLibrary.simpleMessage(
      "Також перенаправляти /from/… і зберігати решту шляху",
    ),
    "appAlsoWwwRedirectsHere": MessageLookupByLibrary.simpleMessage(
      "Також www. (перенаправляє сюди)",
    ),
    "appAnyPublicImageOrA": MessageLookupByLibrary.simpleMessage(
      "Будь-який публічний образ або приватний із входом у Налаштуваннях → Реєстри. Вказуйте тег версії (не :latest), щоб розгортання й відкати були відтворюваними.",
    ),
    "appAuto3000032767": MessageLookupByLibrary.simpleMessage(
      "авто (30000–32767)",
    ),
    "appBackToTheProject": MessageLookupByLibrary.simpleMessage(
      "Назад до проєкту",
    ),
    "appBranch": MessageLookupByLibrary.simpleMessage("Гілка"),
    "appBuildAndDeployOnEvery": MessageLookupByLibrary.simpleMessage(
      "Збирати й розгортати після кожного push",
    ),
    "appBuildDeploy": MessageLookupByLibrary.simpleMessage(
      "Зібрати й розгорнути",
    ),
    "appBuildFromAGitRepository": MessageLookupByLibrary.simpleMessage(
      "Натомість збирати з Git-репозиторію",
    ),
    "appBuildFromGit": MessageLookupByLibrary.simpleMessage("Зібрати з Git"),
    "appBuildStartedItDeploysWhen": m16,
    "appBuilds": MessageLookupByLibrary.simpleMessage("Збірки"),
    "appCancelBuild": MessageLookupByLibrary.simpleMessage("Скасувати збірку"),
    "appCancelUpload": MessageLookupByLibrary.simpleMessage(
      "Скасувати завантаження",
    ),
    "appChangeTheseOnTheSettings": MessageLookupByLibrary.simpleMessage(
      "Змінюйте це на вкладці «Налаштування», а змінні та файли — на вкладці «Середовище».",
    ),
    "appChangedAreSavedButNot": m17,
    "appChangesApplyToTheRunning": MessageLookupByLibrary.simpleMessage(
      "Зміни одразу діють для запущеного застосунку. Нові файли отримують власника своєї теки.",
    ),
    "appChangesInsideTheContainerAre": MessageLookupByLibrary.simpleMessage(
      "Зміни всередині контейнера втрачаються після перезапуску, окрім томів. Кожен сеанс записується в журнал аудиту.",
    ),
    "appChangingSettingsNeedsTheApp": MessageLookupByLibrary.simpleMessage(
      "Для зміни налаштувань потрібен дозвіл app.edit.",
    ),
    "appCommands": MessageLookupByLibrary.simpleMessage("Команди"),
    "appCompressTextResponsesGzip": MessageLookupByLibrary.simpleMessage(
      "Стискати текстові відповіді (gzip)",
    ),
    "appConfigFiles": MessageLookupByLibrary.simpleMessage(
      "Конфігураційні файли",
    ),
    "appConfiguration": MessageLookupByLibrary.simpleMessage("Конфігурація"),
    "appContainerPort": MessageLookupByLibrary.simpleMessage("Порт контейнера"),
    "appContent": MessageLookupByLibrary.simpleMessage("Вміст"),
    "appCopied": m18,
    "appCopyApp": MessageLookupByLibrary.simpleMessage("Копіювати застосунок…"),
    "appCouldNotDiscard": m19,
    "appCpuMillicores": MessageLookupByLibrary.simpleMessage("CPU (мілі-ядра)"),
    "appCrashLog": MessageLookupByLibrary.simpleMessage("Журнал збою"),
    "appDatabaseUrlPostgresLogLevel": MessageLookupByLibrary.simpleMessage(
      "DATABASE_URL=postgres://…\nLOG_LEVEL=info",
    ),
    "appDatabases": MessageLookupByLibrary.simpleMessage("Бази даних"),
    "appDefault": MessageLookupByLibrary.simpleMessage("Типове"),
    "appDefaultDomainWithALet": MessageLookupByLibrary.simpleMessage(
      "типовий домен із сертифікатом Let\'s Encrypt",
    ),
    "appDelete": m20,
    "appDelete2": m21,
    "appDeleteApp": MessageLookupByLibrary.simpleMessage(
      "Видалити застосунок…",
    ),
    "appDeleteData": MessageLookupByLibrary.simpleMessage("Видалити дані"),
    "appDeletePreview": m22,
    "appDeletePreviewHelp": MessageLookupByLibrary.simpleMessage(
      "Новий коміт у pull request створить його знову.",
    ),
    "appDeleteTheDataOf": m23,
    "appDeploy": MessageLookupByLibrary.simpleMessage("Розгорнути"),
    "appDeployANewVersion": MessageLookupByLibrary.simpleMessage(
      "Розгорнути нову версію",
    ),
    "appDeployAnotherImage": MessageLookupByLibrary.simpleMessage(
      "Розгорнути інший образ",
    ),
    "appDeployAnyway": MessageLookupByLibrary.simpleMessage(
      "Все одно розгорнути",
    ),
    "appDeployImage": MessageLookupByLibrary.simpleMessage("Розгорнути образ"),
    "appDeployInProgress": MessageLookupByLibrary.simpleMessage(
      "Триває розгортання",
    ),
    "appDiscard": MessageLookupByLibrary.simpleMessage("Відкинути"),
    "appDisconnect": MessageLookupByLibrary.simpleMessage("Від\'єднатися"),
    "appDockerfile": MessageLookupByLibrary.simpleMessage("Dockerfile"),
    "appDownload": MessageLookupByLibrary.simpleMessage(
      "Завантажити на пристрій",
    ),
    "appEG15Every15": MessageLookupByLibrary.simpleMessage(
      "напр. */15 * * * * = кожні 15 хвилин, @daily",
    ),
    "appEGMigrateUp": MessageLookupByLibrary.simpleMessage(
      "напр. ./migrate up",
    ),
    "appEmptyFolder": MessageLookupByLibrary.simpleMessage("Порожня тека"),
    "appEnvironmentVariables": MessageLookupByLibrary.simpleMessage(
      "Змінні середовища",
    ),
    "appEverythingStoredOnThisVolume": MessageLookupByLibrary.simpleMessage(
      "Усе, що зберігається на цьому томі, буде видалено назавжди.",
    ),
    "appExistingImage": MessageLookupByLibrary.simpleMessage("Наявний образ"),
    "appFiles": MessageLookupByLibrary.simpleMessage("Файли"),
    "appFiles2": m24,
    "appFilesUpTo64Mb": MessageLookupByLibrary.simpleMessage(
      "Сюди можна завантажувати файли до 64 МБ",
    ),
    "appFixedOnceTheVolumeExists": MessageLookupByLibrary.simpleMessage(
      "Не змінюється після створення тому.",
    ),
    "appGeneral": MessageLookupByLibrary.simpleMessage("Загальне"),
    "appGhcrIoAcmeApp1": MessageLookupByLibrary.simpleMessage(
      "ghcr.io/acme/app:1.2.3",
    ),
    "appGhcrIoAcmeShop1": MessageLookupByLibrary.simpleMessage(
      "ghcr.io/acme/shop:1.4.2",
    ),
    "appGithubAFineGrainedToken": MessageLookupByLibrary.simpleMessage(
      "GitHub: fine-grained токен із Contents: read. GitLab: read_repository.",
    ),
    "appGithubSettingsWebhooksAddWebhook": MessageLookupByLibrary.simpleMessage(
      "GitHub: Settings → Webhooks → Add webhook, content type application/json, цей секрет. GitLab: Webhooks, secret token = цей секрет, push events. Gitea/Forgejo: так само, як GitHub. Для середовищ попереднього перегляду надсилайте також події pull request (GitLab: merge request).",
    ),
    "appHiddenInThePanelUnless": MessageLookupByLibrary.simpleMessage(
      "Прихований у панелі, доки його не показати (на диску завжди зашифрований)",
    ),
    "appHide": MessageLookupByLibrary.simpleMessage("Сховати"),
    "appHstsBrowsersOnlyUseHttps": MessageLookupByLibrary.simpleMessage(
      "HSTS (браузери використовують лише HTTPS)",
    ),
    "appHtmlCssJsJsonThat": MessageLookupByLibrary.simpleMessage(
      "HTML, CSS, JS, JSON …, які застосунок не стискає сам",
    ),
    "appHttp": MessageLookupByLibrary.simpleMessage("HTTP"),
    "appHttpsLetSEncrypt": MessageLookupByLibrary.simpleMessage(
      "HTTPS (Let\'s Encrypt)",
    ),
    "appImage": MessageLookupByLibrary.simpleMessage("Образ"),
    "appIncludeSubdomains": MessageLookupByLibrary.simpleMessage(
      "Включно з піддоменами",
    ),
    "appIsNotATextFile": m25,
    "appIsSavedAsNotDeployed": m26,
    "appKey": MessageLookupByLibrary.simpleMessage("KEY"),
    "appKubernetesEvents": MessageLookupByLibrary.simpleMessage(
      "Події Kubernetes",
    ),
    "appLeaveEmptyForTheDefault": MessageLookupByLibrary.simpleMessage(
      "Залиште порожнім для типового значення",
    ),
    "appLeaveTheWizard": MessageLookupByLibrary.simpleMessage(
      "Вийти з майстра?",
    ),
    "appLetOtherSitesCallThis": MessageLookupByLibrary.simpleMessage(
      "Дозволити іншим сайтам звертатися до цього домену з браузера",
    ),
    "appMaxUploadMb": MessageLookupByLibrary.simpleMessage(
      "Макс. завантаження, МБ",
    ),
    "appMemoryMb": MessageLookupByLibrary.simpleMessage("Пам\'ять (МБ)"),
    "appMore": MessageLookupByLibrary.simpleMessage("Більше"),
    "appMoreReplicas": MessageLookupByLibrary.simpleMessage("Більше реплік"),
    "appMountFilesSuchAsApplication": MessageLookupByLibrary.simpleMessage(
      "Монтуйте файли на кшталт application.yml, .env чи сертифіката за будь-яким шляхом у контейнері.",
    ),
    "appMountedAt": MessageLookupByLibrary.simpleMessage("Змонтовано в"),
    "appNameValuePerLineAn": MessageLookupByLibrary.simpleMessage(
      "«Назва: значення» в кожному рядку; порожнє значення прибирає заголовок.",
    ),
    "appNeedsThePushWebhookIn": MessageLookupByLibrary.simpleMessage(
      "Потрібен вебхук push у вашому Git-хостингу",
    ),
    "appNetworkAndHealthCheck": MessageLookupByLibrary.simpleMessage(
      "Мережа й перевірка справності",
    ),
    "appNewFolder": MessageLookupByLibrary.simpleMessage("Нова тека"),
    "appNewOrHttps": MessageLookupByLibrary.simpleMessage("/new або https://…"),
    "appNewSecret": MessageLookupByLibrary.simpleMessage("Новий секрет"),
    "appNewWebhookSecret": MessageLookupByLibrary.simpleMessage(
      "Новий секрет вебхука?",
    ),
    "appNewestPod": MessageLookupByLibrary.simpleMessage("Найновіший под"),
    "appNoBaseDomainIsSet": MessageLookupByLibrary.simpleMessage(
      "Базовий домен не задано, тож застосунок не отримає домен автоматично. Задайте його в Налаштуваннях або додайте домен на сторінці застосунку пізніше.",
    ),
    "appNoBuildYet": MessageLookupByLibrary.simpleMessage("Збірок ще не було."),
    "appNoDatabaseLinkedAddOne": MessageLookupByLibrary.simpleMessage(
      "Базу даних не пов\'язано. Додайте її на сторінці проєкту й пов\'яжіть із цим застосунком.",
    ),
    "appNoDeploymentsYet": MessageLookupByLibrary.simpleMessage(
      "Розгортань ще не було.",
    ),
    "appNoDomainSetABase": MessageLookupByLibrary.simpleMessage(
      "Домену немає. Задайте базовий домен у Налаштуваннях, щоб отримати його автоматично; власні домени — на сторінці доменів.",
    ),
    "appNoLimit": MessageLookupByLibrary.simpleMessage("Без обмеження"),
    "appNoPreviews": MessageLookupByLibrary.simpleMessage(
      "Жоден відкритий pull request не має попереднього перегляду.",
    ),
    "appNoVariables": MessageLookupByLibrary.simpleMessage("Змінних немає."),
    "appNoVariablesEveryAppAlso": MessageLookupByLibrary.simpleMessage(
      "Змінних немає. Кожен застосунок також отримує PORT і APP_ARGS.",
    ),
    "appNoVolumes": MessageLookupByLibrary.simpleMessage("Томів немає."),
    "appNone": MessageLookupByLibrary.simpleMessage("Немає"),
    "appNotDeployedYet": MessageLookupByLibrary.simpleMessage(
      "Ще не розгорнуто",
    ),
    "appOff": MessageLookupByLibrary.simpleMessage("Вимкнено"),
    "appOneIpOrCidrPer": MessageLookupByLibrary.simpleMessage(
      "По одній IP чи CIDR у рядку; решта отримує 403. Порожньо = відкрито для всіх.",
    ),
    "appOnePerLineOrFor": MessageLookupByLibrary.simpleMessage(
      "По одному в рядку, або * для будь-якого сайту (тоді без облікових даних).",
    ),
    "appOpenAPort": MessageLookupByLibrary.simpleMessage("Відкрити порт"),
    "appOpenPortsOnTheServer": MessageLookupByLibrary.simpleMessage(
      "Відкриті порти на сервері",
    ),
    "appOpenPreview": MessageLookupByLibrary.simpleMessage("Відкрити"),
    "appOpenPullRequest": MessageLookupByLibrary.simpleMessage("Pull request"),
    "appOpeningTheVolume": MessageLookupByLibrary.simpleMessage(
      "Відкриття тому…",
    ),
    "appPassedToYourAppAs": MessageLookupByLibrary.simpleMessage(
      "Передається вашому застосунку як \$APP_ARGS",
    ),
    "appPasteAEnvFile": MessageLookupByLibrary.simpleMessage(
      "Вставте файл .env",
    ),
    "appPasteEnv": MessageLookupByLibrary.simpleMessage("Вставити .env"),
    "appPath": MessageLookupByLibrary.simpleMessage("Шлях"),
    "appPathInTheContainer": MessageLookupByLibrary.simpleMessage(
      "Шлях у контейнері",
    ),
    "appPersistentDisksFilesWrittenThere": MessageLookupByLibrary.simpleMessage(
      "Постійні диски: записані туди файли переживають перезапуски й розгортання. Усе інше в контейнері тимчасове.",
    ),
    "appPointItsDnsARecord": MessageLookupByLibrary.simpleMessage(
      "Спершу спрямуйте його DNS (запис A) на цей сервер.",
    ),
    "appPortIsOpenOnThe": m27,
    "appPortOpen": MessageLookupByLibrary.simpleMessage("Порт відкритий"),
    "appPortYourAppListensOn": MessageLookupByLibrary.simpleMessage(
      "Порт, який слухає ваш застосунок",
    ),
    "appPreviewDatabases": MessageLookupByLibrary.simpleMessage(
      "Попередні перегляди використовують бази даних цього застосунку",
    ),
    "appPreviewDatabasesHelp": MessageLookupByLibrary.simpleMessage(
      "Вимкнено: попередні перегляди запускаються без зв\'язків із базами даних (вони не мають змінювати справжні дані).",
    ),
    "appPreviewDays": m28,
    "appPreviewEnvironments": MessageLookupByLibrary.simpleMessage(
      "Середовища попереднього перегляду",
    ),
    "appPreviewForks": MessageLookupByLibrary.simpleMessage(
      "Також pull request із форків",
    ),
    "appPreviewForksHelp": MessageLookupByLibrary.simpleMessage(
      "Їхній код працює зі змінними й секретами цього застосунку.",
    ),
    "appPreviewOf": m29,
    "appPreviewOfHelp": m30,
    "appPreviewTtl": MessageLookupByLibrary.simpleMessage(
      "Видаляти неактивні попередні перегляди через",
    ),
    "appPreviewTtlNever": MessageLookupByLibrary.simpleMessage("ніколи"),
    "appPreviews": MessageLookupByLibrary.simpleMessage("Попередні перегляди"),
    "appPreviewsHelp": MessageLookupByLibrary.simpleMessage(
      "Кожен pull request у цю гілку отримує власну копію застосунку на окремому домені; вона перезбирається після нових комітів і видаляється, коли pull request закрито. Потрібен вебхук із подіями pull request.",
    ),
    "appPreviewsSwitch": MessageLookupByLibrary.simpleMessage(
      "Попередній перегляд для кожного pull request",
    ),
    "appProjectDatabases": MessageLookupByLibrary.simpleMessage(
      "Бази даних проєкту",
    ),
    "appPromoteTo": m31,
    "appPushWebhook": MessageLookupByLibrary.simpleMessage("Вебхук push"),
    "appReady": m32,
    "appRedeploy": MessageLookupByLibrary.simpleMessage("Розгорнути знову"),
    "appRedeployCurrentVersion": MessageLookupByLibrary.simpleMessage(
      "Розгорнути поточну версію знову",
    ),
    "appRedirectHttpToHttps": MessageLookupByLibrary.simpleMessage(
      "Перенаправляти HTTP на HTTPS",
    ),
    "appRedirectsAccessHeaders": MessageLookupByLibrary.simpleMessage(
      "Перенаправлення, доступ, заголовки",
    ),
    "appRemove": m33,
    "appRemove2": m34,
    "appRemoveFromTheAppThe": MessageLookupByLibrary.simpleMessage(
      "Прибрати із застосунку (дані зберігаються, доки ви їх не видалите)",
    ),
    "appRemoveRule": MessageLookupByLibrary.simpleMessage("Прибрати правило"),
    "appRemoveTheSavedTokenPublic": MessageLookupByLibrary.simpleMessage(
      "Прибрати збережений токен (публічний репозиторій)",
    ),
    "appRemovedFromTheAppData": MessageLookupByLibrary.simpleMessage(
      "Прибрано із застосунку, дані ще зберігаються",
    ),
    "appRenameMove": MessageLookupByLibrary.simpleMessage(
      "Перейменувати / перемістити",
    ),
    "appReplace": m35,
    "appReplace2": MessageLookupByLibrary.simpleMessage("Замінити"),
    "appRepository": MessageLookupByLibrary.simpleMessage("Репозиторій"),
    "appRequestHeadersToTheApp": MessageLookupByLibrary.simpleMessage(
      "Заголовки запиту (до застосунку)",
    ),
    "appRequestsSPerIp": MessageLookupByLibrary.simpleMessage(
      "Запитів/с на IP",
    ),
    "appReserveTheseButAllowUsing": MessageLookupByLibrary.simpleMessage(
      "Резервувати це, але дозволити використовувати вільні ресурси",
    ),
    "appResourcesPerReplica": MessageLookupByLibrary.simpleMessage(
      "Ресурси на репліку",
    ),
    "appResponseHeaders": MessageLookupByLibrary.simpleMessage(
      "Заголовки відповіді",
    ),
    "appRestart": MessageLookupByLibrary.simpleMessage("Перезапустити"),
    "appRetryUpload": MessageLookupByLibrary.simpleMessage(
      "Повторити завантаження",
    ),
    "appRevealSecrets": MessageLookupByLibrary.simpleMessage(
      "Показати секрети",
    ),
    "appRoot": MessageLookupByLibrary.simpleMessage("(корінь)"),
    "appRotate": MessageLookupByLibrary.simpleMessage("Замінити"),
    "appSaveFile": MessageLookupByLibrary.simpleMessage("Зберегти файл"),
    "appSaved": m36,
    "appScheduleCron": MessageLookupByLibrary.simpleMessage("Розклад (cron)"),
    "appSecret": MessageLookupByLibrary.simpleMessage("Секрет"),
    "appServerPort": MessageLookupByLibrary.simpleMessage("Порт сервера"),
    "appSharedByAllReplicas": MessageLookupByLibrary.simpleMessage(
      "Спільний для всіх реплік",
    ),
    "appSharedWithProject": MessageLookupByLibrary.simpleMessage(
      "спільна для проєкту",
    ),
    "appShopApi": MessageLookupByLibrary.simpleMessage("Shop API"),
    "appSizeMib": MessageLookupByLibrary.simpleMessage("Розмір (МіБ)"),
    "appSource": MessageLookupByLibrary.simpleMessage("Джерело"),
    "appSpringProfilesActiveProd": MessageLookupByLibrary.simpleMessage(
      "--spring.profiles.active=prod",
    ),
    "appStepOf": m37,
    "appStorage": MessageLookupByLibrary.simpleMessage("Сховище"),
    "appTcp": MessageLookupByLibrary.simpleMessage("TCP"),
    "appTcpUdpPortsReachableFrom": MessageLookupByLibrary.simpleMessage(
      "Порти TCP/UDP, доступні ззовні (MQTT, ігрові сервери…). Вебтрафік натомість іде через домени.",
    ),
    "appTheFileDisappearsFromThe": MessageLookupByLibrary.simpleMessage(
      "Файл зникне з контейнерів після наступного розгортання.",
    ),
    "appTheFileInThisFolder": MessageLookupByLibrary.simpleMessage(
      "Файл у цій теці буде перезаписано.",
    ),
    "appTheGatewayStopsServingIt": MessageLookupByLibrary.simpleMessage(
      "Шлюз перестане його обслуговувати.",
    ),
    "appTheOldSecretStopsWorking": MessageLookupByLibrary.simpleMessage(
      "Старий секрет перестане діяти: оновіть його у своєму Git-хостингу.",
    ),
    "appTheRepositoryNeedsADockerfile": MessageLookupByLibrary.simpleMessage(
      "Репозиторію потрібен Dockerfile. Образи збираються в кластері за допомогою Kaniko і зберігаються у вбудованому реєстрі.",
    ),
    "appTheTypeIsFixedOnce": MessageLookupByLibrary.simpleMessage(
      "Тип не можна змінити після створення застосунку. Відкиньте застосунок, щоб вибрати інший тип.",
    ),
    "appThereAreNoProjectsYet": MessageLookupByLibrary.simpleMessage(
      "Проєктів ще немає. Спершу створіть проєкт.",
    ),
    "appThisCannotBeUndoneBackups": MessageLookupByLibrary.simpleMessage(
      "Скасувати це неможливо (резервні копії тому зберігають старіші версії).",
    ),
    "appThisFileSuitsBetterThan": m38,
    "appThisRemovesTheAppIts": MessageLookupByLibrary.simpleMessage(
      "Буде видалено застосунок, його домени, релізи й завантажені файли з сервера. Скасувати це неможливо.",
    ),
    "appTimeoutS": MessageLookupByLibrary.simpleMessage("Тайм-аут, с"),
    "appTrafficOnlyReachesAReplica": MessageLookupByLibrary.simpleMessage(
      "Трафік потрапляє на репліку лише після успішної перевірки справності, тож зламаний реліз ніколи не замінить робочий.",
    ),
    "appTypeToConfirm": m39,
    "appUdp": MessageLookupByLibrary.simpleMessage("UDP"),
    "appUpload": MessageLookupByLibrary.simpleMessage("Завантажити"),
    "appUploadedAndChecked": m40,
    "appUploading": m41,
    "appUse": m42,
    "appUsed": MessageLookupByLibrary.simpleMessage("Використано"),
    "appUsedInKubernetesNamesAnd": MessageLookupByLibrary.simpleMessage(
      "Використовується в іменах Kubernetes і типовому домені",
    ),
    "appVolumeDeleted": m43,
    "appWasCreatedButNotDeployed": m44,
    "appXEnvStaging": MessageLookupByLibrary.simpleMessage("X-Env: staging"),
    "appXFrameOptionsDeny": MessageLookupByLibrary.simpleMessage(
      "X-Frame-Options: DENY",
    ),
    "appYouCanLeaveThisPage": MessageLookupByLibrary.simpleMessage(
      "Цю сторінку можна залишити; розгортання триває на сервері.",
    ),
    "appYourOwnVariablesWithThe": MessageLookupByLibrary.simpleMessage(
      "Ваші власні змінні з тими самими назвами мають пріоритет.",
    ),
    "authAtLeast10Characters": MessageLookupByLibrary.simpleMessage(
      "Щонайменше 10 символів",
    ),
    "authBeforeThePanelHasA": MessageLookupByLibrary.simpleMessage(
      "Поки в панелі немає домену, відкрийте SSH-тунель і використовуйте http://localhost:8480.",
    ),
    "authCannotReachThePanel": MessageLookupByLibrary.simpleMessage(
      "Не вдається зв\'язатися з панеллю",
    ),
    "authChooseAPassword": MessageLookupByLibrary.simpleMessage(
      "Виберіть пароль",
    ),
    "authCode": MessageLookupByLibrary.simpleMessage("Код"),
    "authConnect": MessageLookupByLibrary.simpleMessage("Під\'єднатися"),
    "authConnectToYourServer": MessageLookupByLibrary.simpleMessage(
      "Під\'єднайтеся до свого сервера",
    ),
    "authContinueWith": m45,
    "authCreateOwnerAccount": MessageLookupByLibrary.simpleMessage(
      "Створити обліковий запис власника",
    ),
    "authEnterThe6DigitCode": MessageLookupByLibrary.simpleMessage(
      "Введіть 6-значний код із застосунку-автентифікатора або один із кодів відновлення.",
    ),
    "authForgotYourPassword": MessageLookupByLibrary.simpleMessage(
      "Забули пароль?",
    ),
    "authGoToSignIn": MessageLookupByLibrary.simpleMessage("Перейти до входу"),
    "authIfTheAddressHasAn": MessageLookupByLibrary.simpleMessage(
      "Якщо для цієї адреси є обліковий запис, посилання для скидання вже в дорозі.",
    ),
    "authLinkExpired": MessageLookupByLibrary.simpleMessage(
      "Термін дії посилання минув",
    ),
    "authNectarAppsRunsOnYour": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps працює на вашому власному сервері. Введіть адресу його панелі.",
    ),
    "authOr": MessageLookupByLibrary.simpleMessage("або"),
    "authPanelAddress": MessageLookupByLibrary.simpleMessage("Адреса панелі"),
    "authPasswordSaved": MessageLookupByLibrary.simpleMessage(
      "Пароль збережено",
    ),
    "authRepeatPassword": MessageLookupByLibrary.simpleMessage(
      "Повторіть пароль",
    ),
    "authRepeatThePassword": MessageLookupByLibrary.simpleMessage(
      "Повторіть пароль",
    ),
    "authResetYourPassword": MessageLookupByLibrary.simpleMessage(
      "Скидання пароля",
    ),
    "authSendLink": MessageLookupByLibrary.simpleMessage("Надіслати посилання"),
    "authSetupToken": MessageLookupByLibrary.simpleMessage(
      "Токен налаштування",
    ),
    "authSignInWithPasskey": MessageLookupByLibrary.simpleMessage(
      "Увійти за допомогою ключа доступу",
    ),
    "authSignInWithYourNew": MessageLookupByLibrary.simpleMessage(
      "Увійдіть із новим паролем і кодом з автентифікатора.",
    ),
    "authTheInstallerPrintedItTogether": MessageLookupByLibrary.simpleMessage(
      "Інсталятор вивів його разом із посиланням для налаштування.",
    ),
    "authUseAnotherServer": MessageLookupByLibrary.simpleMessage(
      "Інший сервер",
    ),
    "authVerify": MessageLookupByLibrary.simpleMessage("Підтвердити"),
    "authWeEmailYouALink": MessageLookupByLibrary.simpleMessage(
      "Ми надішлемо вам посилання для вибору нового пароля (якщо панель може надсилати листи).",
    ),
    "authWelcomeToNectarApps": MessageLookupByLibrary.simpleMessage(
      "Ласкаво просимо до Nectar Apps",
    ),
    "authYourName": MessageLookupByLibrary.simpleMessage("Ваше ім\'я"),
    "backupATargetIsWhereBackups": MessageLookupByLibrary.simpleMessage(
      "Сховище — це місце для резервних копій: бакет S3 (AWS, Backblaze B2, R2, MinIO), сервер SFTP або тека на цьому сервері.",
    ),
    "backupAddAPlanForThe": MessageLookupByLibrary.simpleMessage(
      "Додайте план для самої панелі (потрібен, щоб відновити втрачений сервер), для баз даних і застосунків із томами.",
    ),
    "backupAddATargetFirst": MessageLookupByLibrary.simpleMessage(
      "Спершу додайте сховище",
    ),
    "backupAddPlan": MessageLookupByLibrary.simpleMessage("Додати план"),
    "backupAddTarget": MessageLookupByLibrary.simpleMessage("Додати сховище"),
    "backupBackUpNow": MessageLookupByLibrary.simpleMessage("Скопіювати зараз"),
    "backupBackblazeB2": MessageLookupByLibrary.simpleMessage("Backblaze B2"),
    "backupBackupOfStarted": m46,
    "backupBackupsAreEncryptedWithThe": MessageLookupByLibrary.simpleMessage(
      "Резервні копії шифруються паролем репозиторію ще до того, як залишають сервер.",
    ),
    "backupBackupsOfVolumesReadFiles": MessageLookupByLibrary.simpleMessage(
      "Копіювання томів читає файли, поки застосунок працює; бази даних вивантажуються узгоджено. Час — UTC.",
    ),
    "backupBeginOpensshPrivateKey": MessageLookupByLibrary.simpleMessage(
      "-----BEGIN OPENSSH PRIVATE KEY-----",
    ),
    "backupChangesSinceThenAreLost": m47,
    "backupCheck": MessageLookupByLibrary.simpleMessage("Перевірити"),
    "backupChecking": m48,
    "backupCopied": MessageLookupByLibrary.simpleMessage("Скопійовано"),
    "backupCopy": MessageLookupByLibrary.simpleMessage("Копіювати"),
    "backupCronExpressionUtc": MessageLookupByLibrary.simpleMessage(
      "Вираз cron (UTC)",
    ),
    "backupCustomCron": MessageLookupByLibrary.simpleMessage(
      "Власний розклад (cron)",
    ),
    "backupDeleteThisPlan": MessageLookupByLibrary.simpleMessage(
      "Видалити цей план?",
    ),
    "backupDone": MessageLookupByLibrary.simpleMessage("Готово"),
    "backupEdit": MessageLookupByLibrary.simpleMessage("Змінити"),
    "backupFolder": MessageLookupByLibrary.simpleMessage("Тека"),
    "backupItHoldsTheRepositoryPassword": MessageLookupByLibrary.simpleMessage(
      "Він містить пароль репозиторію та ключі доступу: будь-хто, хто має його та майстер-ключ, може прочитати ваші резервні копії.",
    ),
    "backupKeep": MessageLookupByLibrary.simpleMessage("Зберігати"),
    "backupMinuteHourDayMonthWeekday": MessageLookupByLibrary.simpleMessage(
      "хвилина година день місяць день_тижня",
    ),
    "backupName": MessageLookupByLibrary.simpleMessage("Назва"),
    "backupNext": m49,
    "backupNoBackupHasRunYet": MessageLookupByLibrary.simpleMessage(
      "Резервне копіювання ще не запускалося.",
    ),
    "backupNoMoreBackupsOfTo": m50,
    "backupNoSnapshotsYet": MessageLookupByLibrary.simpleMessage(
      "Знімків ще немає.",
    ),
    "backupNoTargetYet": MessageLookupByLibrary.simpleMessage(
      "Сховищ ще немає.",
    ),
    "backupNotBackedUpYet": MessageLookupByLibrary.simpleMessage(
      "Ще не скопійовано",
    ),
    "backupNothingIsBackedUpYet": MessageLookupByLibrary.simpleMessage(
      "Резервних копій ще немає",
    ),
    "backupOlderSnapshotsAreRemovedAfter": MessageLookupByLibrary.simpleMessage(
      "Старіші знімки видаляються після кожного копіювання.",
    ),
    "backupReadingTheRepository": MessageLookupByLibrary.simpleMessage(
      "Читання репозиторію…",
    ),
    "backupRecoveryKit": MessageLookupByLibrary.simpleMessage(
      "Набір для відновлення",
    ),
    "backupRemove": m51,
    "backupRemove2": MessageLookupByLibrary.simpleMessage("Прибрати"),
    "backupRestore": m52,
    "backupRestore2": MessageLookupByLibrary.simpleMessage("Відновити"),
    "backupRestoreStartedFollowItUnder": MessageLookupByLibrary.simpleMessage(
      "Відновлення розпочато: стежте за ним в Історії",
    ),
    "backupSaveThisAsNectarRecovery": MessageLookupByLibrary.simpleMessage(
      "Збережіть це як nectar-recovery-kit.json у надійному місці разом із майстер-ключем. На новому сервері: install.sh --restore-kit nectar-recovery-kit.json --master-key <key>",
    ),
    "backupScheduledBackupsOn": MessageLookupByLibrary.simpleMessage(
      "Копіювання за розкладом увімкнено",
    ),
    "backupSftp": MessageLookupByLibrary.simpleMessage("SFTP"),
    "backupSnapshots": m53,
    "backupThePanelForgetsTheTarget": MessageLookupByLibrary.simpleMessage(
      "Панель забуде це сховище. Резервні копії залишаться там, де є.",
    ),
    "backupTo": MessageLookupByLibrary.simpleMessage("Куди"),
    "backupVolumesDatabaseDumpsAndThe": MessageLookupByLibrary.simpleMessage(
      "Томи, дампи баз даних і сама панель, зашифровані restic, у S3, Backblaze B2, SFTP або теку.",
    ),
    "backupWhat": MessageLookupByLibrary.simpleMessage("Що"),
    "backupWhatIsBackedUp": MessageLookupByLibrary.simpleMessage(
      "Що резервується",
    ),
    "backupWhen": MessageLookupByLibrary.simpleMessage("Коли"),
    "backupWhereBackupsGo": MessageLookupByLibrary.simpleMessage(
      "Куди зберігаються копії",
    ),
    "backupYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Ваша роль не має дозволу backup.manage.",
    ),
    "countApps": m54,
    "dashboardAProjectIsASpace": MessageLookupByLibrary.simpleMessage(
      "Проєкт — це простір для пов\'язаних застосунків із власною квотою. Створіть проєкт, а потім додайте перший застосунок.",
    ),
    "dashboardContinueSetup": MessageLookupByLibrary.simpleMessage(
      "Продовжити налаштування",
    ),
    "dashboardFinishSettingUpDomainHttps": MessageLookupByLibrary.simpleMessage(
      "Завершіть налаштування: домен, email для HTTPS і приклад застосунку.",
    ),
    "dashboardNeedAttention": MessageLookupByLibrary.simpleMessage(
      "Потребують уваги",
    ),
    "dashboardNoAppsYet": MessageLookupByLibrary.simpleMessage(
      "Застосунків ще немає.",
    ),
    "dashboardNothingDeployedYet": MessageLookupByLibrary.simpleMessage(
      "Ще нічого не розгорнуто",
    ),
    "domainActions": MessageLookupByLibrary.simpleMessage("Дії"),
    "domainAll": MessageLookupByLibrary.simpleMessage("Усі"),
    "domainAutomaticDnsRecordsOff": MessageLookupByLibrary.simpleMessage(
      "Автоматичні записи DNS вимкнено",
    ),
    "domainCheckDns": MessageLookupByLibrary.simpleMessage("Перевірити DNS"),
    "domainCreateDnsRecordsAutomatically": MessageLookupByLibrary.simpleMessage(
      "Створювати записи DNS автоматично",
    ),
    "domainCustom": MessageLookupByLibrary.simpleMessage("Власні"),
    "domainDnsSetup": MessageLookupByLibrary.simpleMessage("Налаштування DNS"),
    "domainEveryDomainOfEveryApp": MessageLookupByLibrary.simpleMessage(
      "Усі домени всіх застосунків, які ви бачите, зі станом HTTPS і DNS.",
    ),
    "domainNoDomainsYet": MessageLookupByLibrary.simpleMessage(
      "Доменів ще немає",
    ),
    "domainNothingMatches": MessageLookupByLibrary.simpleMessage(
      "Нічого не знайдено.",
    ),
    "domainOpenApp": MessageLookupByLibrary.simpleMessage(
      "Відкрити застосунок",
    ),
    "domainPointDomainsAtThisServer": MessageLookupByLibrary.simpleMessage(
      "Спрямуйте домени на цей сервер такими записами у вашого DNS-провайдера:",
    ),
    "domainProblems": MessageLookupByLibrary.simpleMessage("Проблеми"),
    "domainSearchDomainsAppsProjects": MessageLookupByLibrary.simpleMessage(
      "Пошук доменів, застосунків, проєктів",
    ),
    "domainTheRecordServesEveryApp": m55,
    "domainTheServerSPublicIp": MessageLookupByLibrary.simpleMessage(
      "Публічна IP-адреса сервера невідома. Вкажіть її в майстрі налаштування (Налаштування → Домен, email для HTTPS і приклад застосунку), щоб записи можна було показати й перевірити.",
    ),
    "domainWebAppsGetADomain": MessageLookupByLibrary.simpleMessage(
      "Вебзастосунки отримують домен під базовим доменом під час створення; власні домени додайте на сторінці застосунку.",
    ),
    "gateway1Day": MessageLookupByLibrary.simpleMessage("1 день"),
    "gateway1Hour": MessageLookupByLibrary.simpleMessage("1 година"),
    "gateway1Week": MessageLookupByLibrary.simpleMessage("1 тиждень"),
    "gatewayAWildcardCertificateExampleCom": MessageLookupByLibrary.simpleMessage(
      "Wildcard-сертифікат (*.example.com) підтверджується через DNS: шлюз додає запис TXT через вашого DNS-провайдера. Застосунки під ним отримують HTTPS одразу після створення.",
    ),
    "gatewayAboveADomainSRate": MessageLookupByLibrary.simpleMessage(
      "Понад ліміт домену клієнт отримує 429 і може повторити за секунду. Клієнта, який не зупиняється (понад десятикратний ліміт відхилених запитів за хвилину), буде заблоковано:",
    ),
    "gatewayAllDomains": MessageLookupByLibrary.simpleMessage("Усі домени"),
    "gatewayAlsoTheDomainItselfExample": MessageLookupByLibrary.simpleMessage(
      "Також сам домен (example.com)",
    ),
    "gatewayAny": MessageLookupByLibrary.simpleMessage("Будь-який"),
    "gatewayApiToken": MessageLookupByLibrary.simpleMessage("API-токен"),
    "gatewayBlock": MessageLookupByLibrary.simpleMessage("Заблокувати"),
    "gatewayBlock2": m56,
    "gatewayBlockAnIp": MessageLookupByLibrary.simpleMessage("Заблокувати IP"),
    "gatewayBlockIp": MessageLookupByLibrary.simpleMessage("Заблокувати IP"),
    "gatewayBlockedIps": MessageLookupByLibrary.simpleMessage("Заблоковані IP"),
    "gatewayClientIp": MessageLookupByLibrary.simpleMessage("IP клієнта"),
    "gatewayDelete": m57,
    "gatewayDomain": MessageLookupByLibrary.simpleMessage("Домен"),
    "gatewayDomainsOfYourAppsAppear": MessageLookupByLibrary.simpleMessage(
      "Тут з\'являться домени ваших застосунків.",
    ),
    "gatewayExampleCom": MessageLookupByLibrary.simpleMessage("*.example.com"),
    "gatewayFor": MessageLookupByLibrary.simpleMessage("На"),
    "gatewayGet": m58,
    "gatewayHostsItServesFallBack": MessageLookupByLibrary.simpleMessage(
      "Хости, які він обслуговує, повернуться до власних сертифікатів Let\'s Encrypt (окремо для кожного хоста).",
    ),
    "gatewayInCloudflareMyProfileApi": MessageLookupByLibrary.simpleMessage(
      "У Cloudflare: My Profile → API Tokens → Create Token → «Edit zone DNS», обмежений вашою зоною (дозволи Zone:Read і DNS:Edit). Токен зберігається в шлюзі й більше не показується.",
    ),
    "gatewayIpAddress": MessageLookupByLibrary.simpleMessage("IP-адреса"),
    "gatewayLatestRequests": MessageLookupByLibrary.simpleMessage(
      "Останні запити",
    ),
    "gatewayLetSEncryptCertificatesRenew": MessageLookupByLibrary.simpleMessage(
      "Сертифікати Let\'s Encrypt оновлюються автоматично приблизно за 30 днів до завершення; сповіщення «Сертифікат спливає» попередить, якщо оновлення не вдалося.",
    ),
    "gatewayMs": m59,
    "gatewayNectarGatewayServesEveryDomain": MessageLookupByLibrary.simpleMessage(
      "nectar-gateway обслуговує всі домени на портах 80/443: маршрути, сертифікати, обмеження запитів і заблоковані IP.",
    ),
    "gatewayNoRequestsMatchTheGateway": MessageLookupByLibrary.simpleMessage(
      "Немає відповідних запитів. Шлюз тримає останні запити в пам\'яті (ACCESS_LOG_SIZE, типово 2000).",
    ),
    "gatewayNoRoutes": MessageLookupByLibrary.simpleMessage("Маршрутів немає"),
    "gatewayNobodyIsBlockedTheGateway": MessageLookupByLibrary.simpleMessage(
      "Ніхто не заблокований. Шлюз сам блокує сканери (запити до .env, wp-login …).",
    ),
    "gatewayOneCertificateForYourDomain": MessageLookupByLibrary.simpleMessage(
      "Один сертифікат для *.ваш-домен одразу обслуговує всі застосунки, без окремого замовлення Let\'s Encrypt для кожного. Потрібен nectar-gateway 1.2.2 або новіший: оновіть його в Налаштуваннях → Оновлення.",
    ),
    "gatewayOrder": MessageLookupByLibrary.simpleMessage("Замовити"),
    "gatewayOtherDomain": MessageLookupByLibrary.simpleMessage("Інший домен…"),
    "gatewayRateLimits": MessageLookupByLibrary.simpleMessage(
      "Обмеження запитів",
    ),
    "gatewayRateLimitsTimeoutsAndBody": MessageLookupByLibrary.simpleMessage(
      "Обмеження запитів, тайм-аути й розмір тіла задаються для кожного домену на сторінці застосунку (Домени → змінити).",
    ),
    "gatewayReasonOptional": MessageLookupByLibrary.simpleMessage(
      "Причина (необов\'язково)",
    ),
    "gatewayRemoveTheDnsProvider": MessageLookupByLibrary.simpleMessage(
      "Прибрати DNS-провайдера?",
    ),
    "gatewayRenewNow": MessageLookupByLibrary.simpleMessage("Оновити зараз"),
    "gatewayStatus": MessageLookupByLibrary.simpleMessage("Статус"),
    "gatewayTheGatewaySLetS": MessageLookupByLibrary.simpleMessage(
      "Клієнт Let\'s Encrypt шлюзу вимкнено: спершу вкажіть email для Let\'s Encrypt (Налаштування → Шлюз).",
    ),
    "gatewayThisNectarGatewayBlocksA": MessageLookupByLibrary.simpleMessage(
      "Ця версія nectar-gateway блокує клієнта, що перевищив ліміт, на 24 години. Оновіть її, щоб натомість відповідати 429.",
    ),
    "gatewayUnblock": MessageLookupByLibrary.simpleMessage("Розблокувати"),
    "gatewayUntilUnblocked": MessageLookupByLibrary.simpleMessage(
      "до розблокування",
    ),
    "gatewayWildcardCertificate": MessageLookupByLibrary.simpleMessage(
      "Wildcard-сертифікат",
    ),
    "gatewayWildcardCertificates": MessageLookupByLibrary.simpleMessage(
      "Wildcard-сертифікати",
    ),
    "gatewayWildcardCertificatesStopRenewingUntil":
        MessageLookupByLibrary.simpleMessage(
          "Wildcard-сертифікати не оновлюватимуться, доки ви не вкажете його знову.",
        ),
    "gatewayYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Ваша роль не має дозволу server.view.",
    ),
    "goToDashboard": MessageLookupByLibrary.simpleMessage("Перейти на огляд"),
    "languageEnglish": MessageLookupByLibrary.simpleMessage("English"),
    "languageSystem": MessageLookupByLibrary.simpleMessage("Як у системі"),
    "languageUkrainian": MessageLookupByLibrary.simpleMessage("Українська"),
    "loginButton": MessageLookupByLibrary.simpleMessage("Увійти"),
    "loginEmail": MessageLookupByLibrary.simpleMessage("Email"),
    "loginPassword": MessageLookupByLibrary.simpleMessage("Пароль"),
    "loginTitle": MessageLookupByLibrary.simpleMessage("Вхід"),
    "logout": MessageLookupByLibrary.simpleMessage("Вийти"),
    "navAlerts": MessageLookupByLibrary.simpleMessage("Сповіщення"),
    "navBackups": MessageLookupByLibrary.simpleMessage("Резервні копії"),
    "navDashboard": MessageLookupByLibrary.simpleMessage("Огляд"),
    "navDomains": MessageLookupByLibrary.simpleMessage("Домени"),
    "navProjects": MessageLookupByLibrary.simpleMessage("Проєкти"),
    "navServer": MessageLookupByLibrary.simpleMessage("Сервер"),
    "navSettings": MessageLookupByLibrary.simpleMessage("Налаштування"),
    "newApp": MessageLookupByLibrary.simpleMessage("Новий застосунок"),
    "noAccess": MessageLookupByLibrary.simpleMessage("Немає доступу"),
    "observeAllPods": MessageLookupByLibrary.simpleMessage("Усі поди"),
    "observeBeforeTheLastRestart": m60,
    "observeClear": MessageLookupByLibrary.simpleMessage("Очистити"),
    "observeCopiedLines": m61,
    "observeCopiedLines2": m62,
    "observeCopyShownLines": MessageLookupByLibrary.simpleMessage(
      "Копіювати показані рядки",
    ),
    "observeDownloadLast5000Lines": MessageLookupByLibrary.simpleMessage(
      "Завантажити (останні 5000 рядків)",
    ),
    "observeErrors5xx": MessageLookupByLibrary.simpleMessage("Помилки (5xx)"),
    "observeEverythingKept": MessageLookupByLibrary.simpleMessage(
      "Усе збережене",
    ),
    "observeFilter": MessageLookupByLibrary.simpleMessage("Фільтр"),
    "observeHistory": MessageLookupByLibrary.simpleMessage("Історія"),
    "observeLast24Hours": MessageLookupByLibrary.simpleMessage(
      "Останні 24 години",
    ),
    "observeLast6Hours": MessageLookupByLibrary.simpleMessage(
      "Останні 6 годин",
    ),
    "observeLast7Days": MessageLookupByLibrary.simpleMessage("Останні 7 днів"),
    "observeLastHour": MessageLookupByLibrary.simpleMessage("Остання година"),
    "observeLive": MessageLookupByLibrary.simpleMessage("Наживо"),
    "observeNewestFirstLogsAreKept": MessageLookupByLibrary.simpleMessage(
      "Спершу новіші. Журнали зберігаються в панелі стільки днів, скільки задано в Сервер → Диск → Зберігання.",
    ),
    "observeNoRequestsYet": MessageLookupByLibrary.simpleMessage(
      "Запитів ще немає.",
    ),
    "observeNoRequestsYetTrafficAppears": MessageLookupByLibrary.simpleMessage(
      "Запитів ще немає. Трафік з\'явиться, щойно застосунок отримає домен і шлюз почне спрямовувати до нього запити.",
    ),
    "observePerSecond": MessageLookupByLibrary.simpleMessage("За секунду"),
    "observeRegex": MessageLookupByLibrary.simpleMessage(".* regex"),
    "observeRequested": MessageLookupByLibrary.simpleMessage("Запитано"),
    "observeRequests": MessageLookupByLibrary.simpleMessage("Запити"),
    "observeSearch": MessageLookupByLibrary.simpleMessage("Пошук"),
    "observeThePreviousContainerWroteNothing":
        MessageLookupByLibrary.simpleMessage(
          "Попередній контейнер нічого не вивів.",
        ),
    "observeTotalsAcrossReplicasTheDashed": m63,
    "observeTrafficLastHour": MessageLookupByLibrary.simpleMessage(
      "Трафік · остання година",
    ),
    "onboardingAtYourDnsProviderAdd": MessageLookupByLibrary.simpleMessage(
      "У вашого DNS-провайдера додайте один запис:",
    ),
    "onboardingBaseDomain": MessageLookupByLibrary.simpleMessage(
      "Базовий домен",
    ),
    "onboardingContinue": MessageLookupByLibrary.simpleMessage("Продовжити"),
    "onboardingDeploySample": MessageLookupByLibrary.simpleMessage(
      "Розгорнути приклад",
    ),
    "onboardingEmailForLetSEncrypt": MessageLookupByLibrary.simpleMessage(
      "Email для Let\'s Encrypt",
    ),
    "onboardingImage50mCpu64Mb": m64,
    "onboardingLeaveItEmptyToRegister": MessageLookupByLibrary.simpleMessage(
      "Залиште порожнім, щоб зареєструватися без email.",
    ),
    "onboardingOnlyAnOwnerCanSet": MessageLookupByLibrary.simpleMessage(
      "Налаштувати панель може лише власник",
    ),
    "onboardingOnlyUsedForExpiryWarnings": MessageLookupByLibrary.simpleMessage(
      "Використовується лише для попереджень про завершення строку дії",
    ),
    "onboardingSetUpNectarApps": MessageLookupByLibrary.simpleMessage(
      "Налаштування Nectar Apps",
    ),
    "onboardingSkip": MessageLookupByLibrary.simpleMessage("Пропустити"),
    "onboardingSkipSetupForNow": MessageLookupByLibrary.simpleMessage(
      "Поки пропустити налаштування",
    ),
    "onboardingThreeShortStepsYouCan": MessageLookupByLibrary.simpleMessage(
      "Три короткі кроки. Усе можна змінити пізніше в Налаштуваннях.",
    ),
    "oneClickApp": MessageLookupByLibrary.simpleMessage(
      "Застосунок в один клік",
    ),
    "pageNotFound": MessageLookupByLibrary.simpleMessage(
      "Сторінку не знайдено",
    ),
    "projectActivity": MessageLookupByLibrary.simpleMessage("Активність"),
    "projectAdopted": MessageLookupByLibrary.simpleMessage("Перейнято"),
    "projectAdoptedFromAnExistingNamespace": MessageLookupByLibrary.simpleMessage(
      "Перейнято з наявного простору імен: інші навантаження там не змінюються.",
    ),
    "projectAllProjects": MessageLookupByLibrary.simpleMessage("Усі проєкти"),
    "projectAllTemplates": MessageLookupByLibrary.simpleMessage("Усі"),
    "projectApplyNectarYaml": MessageLookupByLibrary.simpleMessage(
      "Застосувати nectar.yaml",
    ),
    "projectApps": MessageLookupByLibrary.simpleMessage("Застосунки"),
    "projectBack": MessageLookupByLibrary.simpleMessage("Назад"),
    "projectChoose": m65,
    "projectCopiedApplyItWithNectar": m66,
    "projectCopiesTheEnvGroupsThe": MessageLookupByLibrary.simpleMessage(
      "Копіюються групи змінних, бази даних (нові й порожні, з новими паролями) та застосунки з налаштуваннями, змінними й конфігураційними файлами. Нічого не розгортається. Згодом «Просунути» розгортає активну збірку застосунку в такий самий застосунок іншого середовища.",
    ),
    "projectCopiesTheSettingsVariablesSecrets":
        MessageLookupByLibrary.simpleMessage(
          "Копіюються налаштування, змінні (і секрети), конфігураційні файли та групи змінних. Томи починаються порожніми; нічого не розгортається.",
        ),
    "projectCopy": m67,
    "projectCpuRequested": MessageLookupByLibrary.simpleMessage(
      "Запитаний CPU",
    ),
    "projectCreate": MessageLookupByLibrary.simpleMessage("Створити"),
    "projectDeleteTheGroup": m68,
    "projectDeployAJarABinary": MessageLookupByLibrary.simpleMessage(
      "Розгорніть JAR, бінарний файл, статичний сайт, Node, Python, PHP або будь-який Docker-образ.",
    ),
    "projectDeployTheApps": MessageLookupByLibrary.simpleMessage(
      "Розгорнути застосунки",
    ),
    "projectEachProjectHasItsOwn": MessageLookupByLibrary.simpleMessage(
      "Кожен проєкт має власний простір імен Kubernetes і необов\'язкову квоту.",
    ),
    "projectEnvGroups": MessageLookupByLibrary.simpleMessage("Групи змінних"),
    "projectEnvironmentOf": m69,
    "projectEnvironmentOfTheCopy": MessageLookupByLibrary.simpleMessage(
      "Середовище копії",
    ),
    "projectExportNectarYaml": MessageLookupByLibrary.simpleMessage(
      "Експортувати nectar.yaml",
    ),
    "projectImport": m70,
    "projectImportDockerComposeYml": MessageLookupByLibrary.simpleMessage(
      "Імпортувати docker-compose.yml",
    ),
    "projectImportExport": MessageLookupByLibrary.simpleMessage(
      "Імпорт / експорт",
    ),
    "projectIntoProject": MessageLookupByLibrary.simpleMessage("У проєкт"),
    "projectInvalidVariableNameUseLetters": m71,
    "projectItsVariablesAreDeleted": MessageLookupByLibrary.simpleMessage(
      "Її змінні буде видалено.",
    ),
    "projectMemoryRequested": MessageLookupByLibrary.simpleMessage(
      "Запитана пам\'ять",
    ),
    "projectMyShop": MessageLookupByLibrary.simpleMessage("Мій магазин"),
    "projectNewEnvironment": MessageLookupByLibrary.simpleMessage(
      "Нове середовище",
    ),
    "projectNewEnvironmentCopy": MessageLookupByLibrary.simpleMessage(
      "Нове середовище (копія)…",
    ),
    "projectNewGroup": MessageLookupByLibrary.simpleMessage("Нова група"),
    "projectNewProject": MessageLookupByLibrary.simpleMessage("Новий проєкт"),
    "projectNoAppsInThisProject": MessageLookupByLibrary.simpleMessage(
      "У цьому проєкті ще немає застосунків",
    ),
    "projectNoProjectsYet": MessageLookupByLibrary.simpleMessage(
      "Проєктів ще немає",
    ),
    "projectOpenTheApp": MessageLookupByLibrary.simpleMessage(
      "Відкрити застосунок",
    ),
    "projectOpenTheDatabase": MessageLookupByLibrary.simpleMessage(
      "Відкрити базу даних",
    ),
    "projectOrPasteItBelow": MessageLookupByLibrary.simpleMessage(
      "або вставте нижче",
    ),
    "projectPasswordsAreGeneratedAndStored": MessageLookupByLibrary.simpleMessage(
      "Паролі генеруються й зберігаються як секретні змінні. Застосунок розгортається одразу.",
    ),
    "projectProject": MessageLookupByLibrary.simpleMessage("Проєкт"),
    "projectProjectSlugOfTheCopy": MessageLookupByLibrary.simpleMessage(
      "Ідентифікатор проєкту-копії",
    ),
    "projectPromote": MessageLookupByLibrary.simpleMessage("Просунути"),
    "projectPromoteTo": m72,
    "projectRequestsOfAllReplicasLive": MessageLookupByLibrary.simpleMessage(
      "Запити всіх реплік. Поточне використання — у перегляді метрик.",
    ),
    "projectResources": MessageLookupByLibrary.simpleMessage("Ресурси"),
    "projectSaved": MessageLookupByLibrary.simpleMessage("Збережено"),
    "projectSearchProjects": MessageLookupByLibrary.simpleMessage(
      "Пошук проєктів",
    ),
    "projectSearchTemplates": MessageLookupByLibrary.simpleMessage(
      "Пошук: MQTT, Kafka, Redis, пошук…",
    ),
    "projectSharedEnvGroups": MessageLookupByLibrary.simpleMessage(
      "Спільні групи змінних",
    ),
    "projectShowSecretValues": MessageLookupByLibrary.simpleMessage(
      "Показати секретні значення",
    ),
    "projectSlug": MessageLookupByLibrary.simpleMessage("Ідентифікатор"),
    "projectTheAppGetsEveryVariable": MessageLookupByLibrary.simpleMessage(
      "Застосунок отримує всі змінні вибраних груп; його власні змінні мають пріоритет.",
    ),
    "projectTheAppSlugItsDefault": MessageLookupByLibrary.simpleMessage(
      "Ідентифікатор застосунку; його типовий домен — <name>-<project>.<base domain>",
    ),
    "projectTheLiveBuildOfIts": m73,
    "projectThisProjectHasNoEnv": MessageLookupByLibrary.simpleMessage(
      "У цьому проєкті ще немає груп змінних (сторінка проєкту → Групи змінних).",
    ),
    "projectVariablesSeveralAppsShareA": MessageLookupByLibrary.simpleMessage(
      "Змінні, спільні для кількох застосунків (регіон, адреса API, Sentry DSN). Застосунки вибирають групи на вкладці «Середовище».",
    ),
    "savedRedeploy": MessageLookupByLibrary.simpleMessage(
      "Збережено. Розгорніть застосунок знову, щоб застосувати.",
    ),
    "serverANectarGatewayReplicaStarts": MessageLookupByLibrary.simpleMessage(
      "Там запуститься репліка nectar-gateway (порти 80 і 443), яка копіює маршрути й сертифікати з основного шлюзу. Записи DNS, якими керує панель, указуватимуть на всі вузли входу. Відкрийте порти 80 і 443 на цій машині.",
    ),
    "serverAddANode": MessageLookupByLibrary.simpleMessage("Додати вузол"),
    "serverAddNode": MessageLookupByLibrary.simpleMessage("Додати вузол"),
    "serverAllowNewPodsUncordon": MessageLookupByLibrary.simpleMessage(
      "Дозволити нові поди (uncordon)",
    ),
    "serverClusterNodesTheirResourcesAnd": MessageLookupByLibrary.simpleMessage(
      "Вузли кластера, їхні ресурси та останні попередження.",
    ),
    "serverCopyCommand": MessageLookupByLibrary.simpleMessage(
      "Копіювати команду",
    ),
    "serverCpu": MessageLookupByLibrary.simpleMessage("CPU"),
    "serverDeletesTheContainerImagesThat": MessageLookupByLibrary.simpleMessage(
      "Видаляє образи контейнерів, які зараз не використовує жоден контейнер на сервері: старі релізи, зупинені застосунки та заздалегідь завантажені образи. За потреби їх буде завантажено знову, тож наступне розгортання або запуск цих застосунків триватиме довше.",
    ),
    "serverDisk": MessageLookupByLibrary.simpleMessage("Диск"),
    "serverDrain": m74,
    "serverDrain2": MessageLookupByLibrary.simpleMessage("Звільнити"),
    "serverDrainAnyway": m75,
    "serverDrainAnyway2": MessageLookupByLibrary.simpleMessage(
      "Все одно звільнити",
    ),
    "serverDrainMovePodsAway": MessageLookupByLibrary.simpleMessage(
      "Звільнити (перенести поди)",
    ),
    "serverExtraImages": MessageLookupByLibrary.simpleMessage(
      "Додаткові образи",
    ),
    "serverGateway": MessageLookupByLibrary.simpleMessage("Шлюз"),
    "serverHistoryLast": m76,
    "serverImageCleanupStarted": MessageLookupByLibrary.simpleMessage(
      "Очищення образів розпочато",
    ),
    "serverImageCleanups": MessageLookupByLibrary.simpleMessage(
      "Очищення образів",
    ),
    "serverIn": MessageLookupByLibrary.simpleMessage("Вхід"),
    "serverItsGatewayReplicaStopsAnd": MessageLookupByLibrary.simpleMessage(
      "Його репліка шлюзу зупиниться, а панель прибере вузол із записів DNS, якими керує. Записи, створені вами вручну, і далі вказуватимуть туди.",
    ),
    "serverK3sDataDirectory": MessageLookupByLibrary.simpleMessage(
      "Тека даних k3s",
    ),
    "serverKeepAppLogs": MessageLookupByLibrary.simpleMessage(
      "Зберігати журнали застосунків",
    ),
    "serverKeepImagesOnEveryNode": MessageLookupByLibrary.simpleMessage(
      "Тримати образи на кожному вузлі",
    ),
    "serverKeptTheLastUploadsPer": m77,
    "serverLiveUsageNeedsMetricsServer": MessageLookupByLibrary.simpleMessage(
      "Для поточного використання потрібен metrics-server (k3s його містить).",
    ),
    "serverMakeAnEdgeNode": m78,
    "serverMakeEdgeNode": MessageLookupByLibrary.simpleMessage(
      "Зробити вузлом входу",
    ),
    "serverMemory": MessageLookupByLibrary.simpleMessage("Пам\'ять"),
    "serverNoNewPodsCordon": MessageLookupByLibrary.simpleMessage(
      "Без нових подів (cordon)",
    ),
    "serverNoNewPodsStartThere": MessageLookupByLibrary.simpleMessage(
      "Нові поди там не запускатимуться, а наявні перейдуть на інші вузли (поди DaemonSet залишаться). Застосунки з однією реплікою будуть недоступні на короткий час.",
    ),
    "serverNoVolumesYetAppsGet": MessageLookupByLibrary.simpleMessage(
      "Томів ще немає. Застосунки отримують їх у Налаштуваннях → Томи; бази даних мають власні.",
    ),
    "serverNodeActions": MessageLookupByLibrary.simpleMessage("Дії з вузлом"),
    "serverOnLocalPathStorageThe": MessageLookupByLibrary.simpleMessage(
      "На сховищі local-path (типове для k3s) розмір не обмежується: том може перевищити його, доки не заповниться диск вузла. Сповіщення «Том майже заповнений» попереджає на 90 %.",
    ),
    "serverOnePerLineEG": MessageLookupByLibrary.simpleMessage(
      "По одному в рядку, напр. postgres:17 (до 20)",
    ),
    "serverOut": MessageLookupByLibrary.simpleMessage("Вихід"),
    "serverPanelDataVolume": MessageLookupByLibrary.simpleMessage(
      "Том даних панелі",
    ),
    "serverPrePullsBusyboxAndThe": MessageLookupByLibrary.simpleMessage(
      "Заздалегідь завантажує busybox і образи середовищ ваших застосунків, щоб нові вузли та переміщені поди запускалися без завантаження. Очищення образів їх не чіпає.",
    ),
    "serverRemoveFromTheCluster": m79,
    "serverRemoveFromTheCluster2": MessageLookupByLibrary.simpleMessage(
      "Видалити з кластера",
    ),
    "serverRemoveImages": MessageLookupByLibrary.simpleMessage(
      "Видалити образи",
    ),
    "serverRemoveUnusedImages": MessageLookupByLibrary.simpleMessage(
      "Видалити невикористані образи?",
    ),
    "serverRetention": MessageLookupByLibrary.simpleMessage("Зберігання"),
    "serverRunThisAsRootOn": MessageLookupByLibrary.simpleMessage(
      "Виконайте це від root на новій машині (Linux, чиста інсталяція):",
    ),
    "serverServeTrafficTooEdgeNode": MessageLookupByLibrary.simpleMessage(
      "Обслуговувати трафік теж (вузол входу)",
    ),
    "serverServerDisk": MessageLookupByLibrary.simpleMessage("Диск сервера"),
    "serverStop": MessageLookupByLibrary.simpleMessage("Зупинити"),
    "serverStopServingTrafficEdge": MessageLookupByLibrary.simpleMessage(
      "Припинити обслуговувати трафік (вхід)",
    ),
    "serverStopServingTrafficOn": m80,
    "serverStoresEveryAppSOutput": MessageLookupByLibrary.simpleMessage(
      "Зберігає вивід кожного застосунку на диску панелі; на вкладці «Журнали» його можна шукати й після перезапусків і розгортань.",
    ),
    "serverTheCommandContainsTheCluster": MessageLookupByLibrary.simpleMessage(
      "Команда містить токен приєднання до кластера: зберігайте її як пароль.",
    ),
    "serverTheNodeObjectIsDeleted": MessageLookupByLibrary.simpleMessage(
      "Об\'єкт вузла буде видалено. На самій машині виконайте /usr/local/bin/k3s-agent-uninstall.sh, щоб прибрати k3s.",
    ),
    "serverUsageShowsWhileTheApp": m81,
    "serverUsedToFindCrictlFor": MessageLookupByLibrary.simpleMessage(
      "Потрібна, щоб знайти crictl для очищення образів",
    ),
    "serverVolumes": MessageLookupByLibrary.simpleMessage("Томи"),
    "serverWarnings": MessageLookupByLibrary.simpleMessage("Попередження"),
    "serverYourRoleDoesNotInclude": MessageLookupByLibrary.simpleMessage(
      "Ваша роль не дозволяє переглядати сервер (server.view).",
    ),
    "setting1123456789Android": MessageLookupByLibrary.simpleMessage(
      "1:123456789:android:…",
    ),
    "setting1123456789Ios": MessageLookupByLibrary.simpleMessage(
      "1:123456789:ios:…",
    ),
    "setting1CreateAFirebaseProject": MessageLookupByLibrary.simpleMessage(
      "1. Створіть проєкт Firebase (console.firebase.google.com) і додайте застосунок Android та/або Apple з package name / bundle id застосунку Nectar.\n2. iPhone і Mac: завантажте свій ключ APNs у Project settings → Cloud Messaging.\n3. Скопіюйте значення нижче з Project settings, потім додайте канал «Мобільний застосунок» у розділі «Сповіщення».",
    ),
    "setting1ScanThisCodeWith": MessageLookupByLibrary.simpleMessage(
      "1. Відскануйте цей код застосунком-автентифікатором.",
    ),
    "setting2EnterThe6Digit": MessageLookupByLibrary.simpleMessage(
      "2. Введіть 6-значний код, який він покаже.",
    ),
    "settingAbout": MessageLookupByLibrary.simpleMessage("Про програму"),
    "settingAccount": MessageLookupByLibrary.simpleMessage("Обліковий запис"),
    "settingActiveSince": m82,
    "settingAddLogin": MessageLookupByLibrary.simpleMessage("Додати вхід"),
    "settingAddPasskey": MessageLookupByLibrary.simpleMessage(
      "Додати ключ доступу",
    ),
    "settingAddProvider": MessageLookupByLibrary.simpleMessage(
      "Додати провайдера",
    ),
    "settingAiza": MessageLookupByLibrary.simpleMessage("AIza…"),
    "settingAllowOnlyTheseIpAddresses": m83,
    "settingAllowedEmailDomains": MessageLookupByLibrary.simpleMessage(
      "Дозволені домени email",
    ),
    "settingApiKey": MessageLookupByLibrary.simpleMessage("API-ключ"),
    "settingAppId": MessageLookupByLibrary.simpleMessage("App ID"),
    "settingAppliedAtEverySignIn": MessageLookupByLibrary.simpleMessage(
      "Застосовується під час кожного входу за claim «groups» від провайдера.",
    ),
    "settingAppsCanNoLongerPull": MessageLookupByLibrary.simpleMessage(
      "Застосунки більше не зможуть завантажувати з нього приватні образи.",
    ),
    "settingAsInXcodeNeededFor": MessageLookupByLibrary.simpleMessage(
      "як у Xcode (потрібен для iPhone)",
    ),
    "settingBundleId": MessageLookupByLibrary.simpleMessage("Bundle ID"),
    "settingButtonLabel": MessageLookupByLibrary.simpleMessage(
      "Напис на кнопці",
    ),
    "settingCallbackRedirectUrlToRegister":
        MessageLookupByLibrary.simpleMessage(
          "Callback (redirect) URL, який треба зареєструвати в провайдера:",
        ),
    "settingChanged": m84,
    "settingClientId": MessageLookupByLibrary.simpleMessage("Client ID"),
    "settingClientSecret": MessageLookupByLibrary.simpleMessage(
      "Client secret",
    ),
    "settingCompanySso": MessageLookupByLibrary.simpleMessage("SSO компанії"),
    "settingCreateUsersOnTheirFirst": MessageLookupByLibrary.simpleMessage(
      "Створювати користувачів під час першого входу",
    ),
    "settingDomainHttpsEmailAndSample": MessageLookupByLibrary.simpleMessage(
      "Домен, email для HTTPS і приклад застосунку (майстер налаштування)",
    ),
    "settingEachCodeSignsYouIn": MessageLookupByLibrary.simpleMessage(
      "Кожен код дає змогу один раз увійти, якщо телефона немає під рукою. Збережіть їх у менеджері паролів: вони показуються лише зараз.",
    ),
    "settingEmailSettingsSaved": MessageLookupByLibrary.simpleMessage(
      "Налаштування пошти збережено",
    ),
    "settingEmptyOnlyUsersThatAlready": MessageLookupByLibrary.simpleMessage(
      "Порожньо: так можуть входити лише наявні користувачі (за email).",
    ),
    "settingExampleComExampleOrg": MessageLookupByLibrary.simpleMessage(
      "example.com, example.org",
    ),
    "settingForTheNectarCliGithub": MessageLookupByLibrary.simpleMessage(
      "Для nectar CLI, GitHub Actions і скриптів",
    ),
    "settingGhcrATokenWithRead": MessageLookupByLibrary.simpleMessage(
      "GHCR: токен із read:packages",
    ),
    "settingGhcrIoDockerIoRegistry": MessageLookupByLibrary.simpleMessage(
      "ghcr.io, docker.io, registry.gitlab.com",
    ),
    "settingGroupRoleOnePerLine": MessageLookupByLibrary.simpleMessage(
      "Група → роль (по одній у рядку)",
    ),
    "settingISavedThem": MessageLookupByLibrary.simpleMessage("Я їх зберіг"),
    "settingId": MessageLookupByLibrary.simpleMessage("ID"),
    "settingInvitePeopleAndGiveThem": MessageLookupByLibrary.simpleMessage(
      "Запрошуйте людей і надавайте їм ролі — всюди або для окремих проєктів",
    ),
    "settingIssuerUrl": MessageLookupByLibrary.simpleMessage(
      "URL видавця (issuer)",
    ),
    "settingItsButtonDisappearsFromThe": MessageLookupByLibrary.simpleMessage(
      "Кнопка зникне зі сторінки входу. Облікові записи користувачів залишаться.",
    ),
    "settingLetPeopleSignInWith": MessageLookupByLibrary.simpleMessage(
      "Дозвольте входити через GitHub, Google або провайдера ідентичності вашої компанії (Authentik, Keycloak, Zitadel, Okta, Entra ID … через OpenID Connect). Працює у вебпанелі.",
    ),
    "settingLicenses": MessageLookupByLibrary.simpleMessage("Ліцензії"),
    "settingLoginsAreStoredEncryptedAnd": MessageLookupByLibrary.simpleMessage(
      "Входи зберігаються зашифрованими й передаються кожному проєкту як image pull secret; збірки використовують їх для базових образів.",
    ),
    "settingNectarAdminsAdminDevelopersDeveloper":
        MessageLookupByLibrary.simpleMessage(
          "nectar-admins=admin\ndevelopers=developer",
        ),
    "settingNectarAppsPanelExampleCom": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps <panel@example.com>",
    ),
    "settingNectarAppsTechbrainLlcNectar": MessageLookupByLibrary.simpleMessage(
      "Nectar Apps · © TechBrain LLC · Nectar Source License",
    ),
    "settingNeedsAllowedDomains": MessageLookupByLibrary.simpleMessage(
      "Потрібні дозволені домени.",
    ),
    "settingNewRecoveryCodes": MessageLookupByLibrary.simpleMessage(
      "Нові коди відновлення",
    ),
    "settingNoEncryption": MessageLookupByLibrary.simpleMessage(
      "Без шифрування",
    ),
    "settingNoPrivateRegistryLoginsPublic": MessageLookupByLibrary.simpleMessage(
      "Входів до приватних реєстрів немає. Для публічних образів (Docker Hub, публічні пакети GHCR, …) вони не потрібні.",
    ),
    "settingNoneAnAdminAssignsRoles": MessageLookupByLibrary.simpleMessage(
      "Жодної (ролі призначає адміністратор)",
    ),
    "settingOnlyRecoveryCodesLeftCreate": m85,
    "settingOnlyTheListedAddressesMay": MessageLookupByLibrary.simpleMessage(
      "Користуватися панеллю можуть лише вказані адреси",
    ),
    "settingOptionalAlertsReachTheNectar": MessageLookupByLibrary.simpleMessage(
      "Необов\'язково. Сповіщення надходять у застосунок Nectar на Android, iPhone і Mac через ваш власний проєкт Firebase (безплатно). Без нього використовуйте канали ntfy, Telegram або email у розділі «Сповіщення».",
    ),
    "settingOrEnterTheKeyBy": MessageLookupByLibrary.simpleMessage(
      "Або введіть ключ вручну:",
    ),
    "settingOwner": MessageLookupByLibrary.simpleMessage("Власник"),
    "settingPanelAccess": MessageLookupByLibrary.simpleMessage(
      "Доступ до панелі",
    ),
    "settingPanelGatewayAndK3sVersions": MessageLookupByLibrary.simpleMessage(
      "Версії панелі, шлюзу та k3s, автоматичні оновлення, історія і відкат",
    ),
    "settingPasskeyAdded": MessageLookupByLibrary.simpleMessage(
      "Ключ доступу додано: увійдіть із ним на сторінці входу",
    ),
    "settingPasskeyDelete": m86,
    "settingPasskeyDeleteHelp": MessageLookupByLibrary.simpleMessage(
      "Ним більше не можна буде увійти сюди. Видаліть його також зі свого пристрою чи менеджера паролів.",
    ),
    "settingPasskeyName": MessageLookupByLibrary.simpleMessage(
      "Новий ключ доступу",
    ),
    "settingPasskeyNameHint": MessageLookupByLibrary.simpleMessage(
      "Назва (напр. MacBook, iPhone, YubiKey)",
    ),
    "settingPasskeyNeverUsed": MessageLookupByLibrary.simpleMessage(
      "ще не використовувався",
    ),
    "settingPasskeySynced": MessageLookupByLibrary.simpleMessage(
      "синхронізований",
    ),
    "settingPasskeyUsed": m87,
    "settingPasskeys": MessageLookupByLibrary.simpleMessage("Ключі доступу"),
    "settingPasskeysHelp": MessageLookupByLibrary.simpleMessage(
      "Входьте за допомогою Touch ID, Face ID, Windows Hello, телефона або ключа безпеки — без пароля й коду. Ключ доступу працює лише за цією адресою панелі.",
    ),
    "settingPasskeysWebOnly": MessageLookupByLibrary.simpleMessage(
      "Додавайте й використовуйте ключі доступу у вебпанелі (свіжий Chrome, Edge, Safari або Firefox).",
    ),
    "settingPasswordOrAccessToken": MessageLookupByLibrary.simpleMessage(
      "Пароль або токен доступу",
    ),
    "settingPermissionSetsCloneABuilt": MessageLookupByLibrary.simpleMessage(
      "Набори дозволів; клонуйте вбудовану роль, щоб створити власну",
    ),
    "settingPlansTargetsRestoreAndThe": MessageLookupByLibrary.simpleMessage(
      "Плани, сховища, відновлення і набір для відновлення",
    ),
    "settingProjectSettingsCloudMessaging":
        MessageLookupByLibrary.simpleMessage(
          "Project settings → Cloud Messaging",
        ),
    "settingProjectSettingsServiceAccountsGenerate":
        MessageLookupByLibrary.simpleMessage(
          "Project settings → Service accounts → Generate new private key. Зберігається зашифрованим.",
        ),
    "settingPushNotifications": MessageLookupByLibrary.simpleMessage(
      "Push-сповіщення",
    ),
    "settingRecoveryCodes": MessageLookupByLibrary.simpleMessage(
      "Коди відновлення",
    ),
    "settingRegistries": MessageLookupByLibrary.simpleMessage("Реєстри"),
    "settingRegistry": MessageLookupByLibrary.simpleMessage("Реєстр"),
    "settingRegistryLogin": MessageLookupByLibrary.simpleMessage(
      "Вхід до реєстру",
    ),
    "settingRemove": m88,
    "settingRemoveTheLoginFor": m89,
    "settingRoleOfNewUsersEverywhere": MessageLookupByLibrary.simpleMessage(
      "Роль нових користувачів (усюди)",
    ),
    "settingRulesAndNotificationChannels": MessageLookupByLibrary.simpleMessage(
      "Правила й канали сповіщень",
    ),
    "settingSaveAndSendATest": MessageLookupByLibrary.simpleMessage(
      "Зберегти й надіслати тест",
    ),
    "settingSendPushNotifications": MessageLookupByLibrary.simpleMessage(
      "Надсилати push-сповіщення",
    ),
    "settingSendTestToMyDevices": MessageLookupByLibrary.simpleMessage(
      "Надіслати тест на мої пристрої",
    ),
    "settingSenderId": MessageLookupByLibrary.simpleMessage("Sender ID"),
    "settingServiceAccountKeyJson": MessageLookupByLibrary.simpleMessage(
      "Ключ сервісного облікового запису (JSON)",
    ),
    "settingSetUp": MessageLookupByLibrary.simpleMessage("Налаштувати"),
    "settingSetUpTwoFactorAuthentication": MessageLookupByLibrary.simpleMessage(
      "Налаштування двофакторної автентифікації",
    ),
    "settingShowOnTheLoginPage": MessageLookupByLibrary.simpleMessage(
      "Показувати на сторінці входу",
    ),
    "settingSignInProviders": MessageLookupByLibrary.simpleMessage(
      "Провайдери входу",
    ),
    "settingSignedIn": MessageLookupByLibrary.simpleMessage("Ви ввійшли"),
    "settingStarttls": MessageLookupByLibrary.simpleMessage("STARTTLS"),
    "settingTestEmailSentToYour": MessageLookupByLibrary.simpleMessage(
      "Тестовий лист надіслано на вашу адресу",
    ),
    "settingTls": MessageLookupByLibrary.simpleMessage("TLS"),
    "settingTurnOff": MessageLookupByLibrary.simpleMessage("Вимкнути"),
    "settingTwoFactorAuthentication": MessageLookupByLibrary.simpleMessage(
      "Двофакторна автентифікація",
    ),
    "settingTwoFactorAuthenticationIsOff": MessageLookupByLibrary.simpleMessage(
      "Двофакторну автентифікацію вимкнено",
    ),
    "settingUsedForPasswordResetsInvitations":
        MessageLookupByLibrary.simpleMessage(
          "Для скидання паролів, запрошень і листів-сповіщень.",
        ),
    "settingUserName": MessageLookupByLibrary.simpleMessage(
      "Ім\'я користувача",
    ),
    "settingYourPassword": MessageLookupByLibrary.simpleMessage("Ваш пароль"),
    "settingsAppearance": MessageLookupByLibrary.simpleMessage("Вигляд"),
    "settingsLanguage": MessageLookupByLibrary.simpleMessage("Мова"),
    "settingsLanguageHint": MessageLookupByLibrary.simpleMessage(
      "Мова інтерфейсу на цьому пристрої. Повідомлення сервера та логи залишаються англійською.",
    ),
    "shellClockAhead": m90,
    "shellClockBehind": m91,
    "shellLater": MessageLookupByLibrary.simpleMessage("Пізніше"),
    "shellReload": MessageLookupByLibrary.simpleMessage("Перезавантажити"),
    "statusDeploying": MessageLookupByLibrary.simpleMessage("Розгортається"),
    "statusFailed": MessageLookupByLibrary.simpleMessage("Помилка"),
    "statusRunning": MessageLookupByLibrary.simpleMessage("Працює"),
    "statusStopped": MessageLookupByLibrary.simpleMessage("Зупинено"),
    "themeDark": MessageLookupByLibrary.simpleMessage("Темна"),
    "themeLight": MessageLookupByLibrary.simpleMessage("Світла"),
    "themeSystem": MessageLookupByLibrary.simpleMessage("Як у системі"),
    "updateAllowAgain": MessageLookupByLibrary.simpleMessage("Дозволити знову"),
    "updateAutomaticUpdates": MessageLookupByLibrary.simpleMessage(
      "Автоматичні оновлення",
    ),
    "updateBeta": MessageLookupByLibrary.simpleMessage("Бета"),
    "updateChannel": MessageLookupByLibrary.simpleMessage("Канал"),
    "updateCheckEvery": MessageLookupByLibrary.simpleMessage(
      "Перевіряти кожні",
    ),
    "updateCheckNow": MessageLookupByLibrary.simpleMessage("Перевірити зараз"),
    "updateCheckedForUpdates": MessageLookupByLibrary.simpleMessage(
      "Оновлення перевірено",
    ),
    "updateEachStepTheDatabaseIs": MessageLookupByLibrary.simpleMessage(
      "На кожному кроці база даних резервується, образ перемикається, і панель чекає, доки нова версія стане справною (а шлюз обслуговує свій канарковий маршрут і зберігає всі маршрути). Якщо перевірка не проходить, усе відкочується автоматично.",
    ),
    "updateEmptyTheOfficialFeedsWhen": MessageLookupByLibrary.simpleMessage(
      "Порожньо = офіційні стрічки. Якщо стрічку не вдається прочитати, панель звертається до GitHub Releases API.",
    ),
    "updateFrom": MessageLookupByLibrary.simpleMessage("З"),
    "updateGatewayFeedUrl": MessageLookupByLibrary.simpleMessage(
      "URL стрічки шлюзу",
    ),
    "updateK3sUpgradeToStartedNode": m92,
    "updateMajorAndBreakingReleasesAre": MessageLookupByLibrary.simpleMessage(
      "Мажорні та несумісні релізи ніколи не встановлюються автоматично. Реліз, що не пройшов перевірки, не повторюється.",
    ),
    "updateNoDaysAndNoTimes": MessageLookupByLibrary.simpleMessage(
      "Без днів і часу = будь-коли. Вікно на кшталт 22:00–02:00 переходить через північ. Оновлення k3s використовують те саме вікно.",
    ),
    "updateNoUpdateHasRunOn": MessageLookupByLibrary.simpleMessage(
      "На цьому сервері ще не було оновлень.",
    ),
    "updatePanelFeedUrl": MessageLookupByLibrary.simpleMessage(
      "URL стрічки панелі",
    ),
    "updatePolicy": MessageLookupByLibrary.simpleMessage("Політика"),
    "updateRollBack": m93,
    "updateRollBack2": MessageLookupByLibrary.simpleMessage("Відкотити"),
    "updateStable": MessageLookupByLibrary.simpleMessage("Стабільний"),
    "updateStableLatestV131": MessageLookupByLibrary.simpleMessage(
      "stable, latest, v1.31",
    ),
    "updateThePanelNectarGatewayAnd": MessageLookupByLibrary.simpleMessage(
      "Панель, nectar-gateway і k3s. Перед кожним оновленням створюється резервна копія, після перемикання — перевірка, а якщо вона не проходить — відкат.",
    ),
    "updateThePanelRestartsNowThis": MessageLookupByLibrary.simpleMessage(
      "Панель зараз перезапуститься: сторінка перепід\'єднається сама за кілька секунд.",
    ),
    "updateTheSystemUpgradeControllerUpgrades":
        MessageLookupByLibrary.simpleMessage(
          "system-upgrade-controller оновлює вузли по одному: вузол отримує cordon, k3s перезапускається з новою версією, потім uncordon. Поди на вузлі перезапускаються під час його оновлення; на сервері з одним вузлом застосунки на короткий час недоступні.",
        ),
    "updateTimeZone": MessageLookupByLibrary.simpleMessage("Часовий пояс"),
    "updateUpdate": m94,
    "updateUpdate2": MessageLookupByLibrary.simpleMessage("Оновити…"),
    "updateUpdateK3sTo": m95,
    "updateUpdateSettingsSaved": MessageLookupByLibrary.simpleMessage(
      "Налаштування оновлень збережено",
    ),
    "updateUpdateSources": MessageLookupByLibrary.simpleMessage(
      "Джерела оновлень",
    ),
    "updateUpdateStarted": MessageLookupByLibrary.simpleMessage(
      "Оновлення розпочато",
    ),
    "updateUpdates": MessageLookupByLibrary.simpleMessage("Оновлення"),
    "updateUpdating": m96,
    "updateUpdatingK3sFromHereNeeds": MessageLookupByLibrary.simpleMessage(
      "Щоб оновлювати k3s звідси, потрібен system-upgrade-controller: повторно запустіть install.sh з --k3s-upgrades.",
    ),
    "updateUpgrade": MessageLookupByLibrary.simpleMessage("Оновити"),
    "updateUtcEuropeKyiv": MessageLookupByLibrary.simpleMessage(
      "UTC, Europe/Kyiv",
    ),
    "updateVersion": MessageLookupByLibrary.simpleMessage("Версія"),
    "updateVersions": MessageLookupByLibrary.simpleMessage("Версії"),
    "widgetLoading": MessageLookupByLibrary.simpleMessage("Завантаження"),
    "widgetNoDataYetHistoryFills": MessageLookupByLibrary.simpleMessage(
      "Даних ще немає: історія оновлюється кожні 15 секунд.",
    ),
    "widgetNothingYet": MessageLookupByLibrary.simpleMessage("Поки нічого."),
    "widgetRetry": MessageLookupByLibrary.simpleMessage("Повторити"),
  };
}
