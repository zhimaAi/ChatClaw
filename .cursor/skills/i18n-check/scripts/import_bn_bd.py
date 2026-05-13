#!/usr/bin/env python3
"""Import Bengali translations."""
import os, re, json

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

def parse_ts_file(content):
    code = re.sub(r'^export\s+default\s*', '', content.strip())
    code = re.sub(r';\s*$', '', code)
    result = {}
    lines = code.split('\n')
    current_path = []
    for line in lines:
        indent = len(line) - len(line.lstrip())
        if not line.strip() or line.strip().startswith('//'):
            continue
        rel_indent = indent // 2
        while len(current_path) > rel_indent:
            current_path.pop()
        obj_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*\{', line)
        if obj_match:
            current_path.append(obj_match.group(1))
            continue
        value_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*', line)
        if value_match:
            key = value_match.group(1)
            rest = line[value_match.end():].strip()
            if rest.startswith('"'):
                value = parse_dq(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
            elif rest.startswith("'"):
                value = parse_sq(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
        if line.strip() in ('}', '},'):
            if current_path:
                current_path.pop()
    return result

def parse_dq(s):
    if not s.startswith('"'): return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i+1]
            if n == 'n': result.append('\n')
            elif n == 'r': result.append('\r')
            elif n == 't': result.append('\t')
            elif n == '\\': result.append('\\')
            elif n == '"': result.append('"')
            elif n == "'": result.append("'")
            else: result.append(s[i:i+2])
            i += 2
        elif s[i] == '"':
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)

def parse_sq(s):
    if not s.startswith("'"): return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i+1]
            if n == "'": result.append("'")
            elif n == '\\': result.append('\\')
            else: result.append(s[i:i+2])
            i += 2
        elif s[i] == "'":
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)

def to_nested_format(flat_obj):
    lines = ['export default {']
    nested = {}
    for key, value in flat_obj.items():
        parts = key.split('.')
        current = nested
        for i, part in enumerate(parts[:-1]):
            if part not in current: current[part] = {}
            if isinstance(current[part], str): current[part] = {'_value': current[part]}
            current = current[part]
        if parts[-1] in current and isinstance(current[parts[-1]], dict) and not isinstance(value, dict):
            current[parts[-1]]['_value'] = value
        else:
            current[parts[-1]] = value
    def esc(v): return str(v).replace("'", '"')
    def po(obj, indent=1):
        is2 = '  ' * indent
        for k, v in obj.items():
            if k == '_value': continue
            if isinstance(v, dict):
                if '_value' in v and len(v) == 1:
                    lines.append(f"{is2}{k}: '{esc(v['_value'])}',")
                else:
                    lines.append(f"{is2}{k}: {{")
                    po(v, indent+1)
                    lines.append(f"{is2}}},")
            else:
                lines.append(f"{is2}{k}: '{esc(v)}',")
    po(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

def load_ts_file(p):
    with open(p, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())

def save_ts_file(p, obj):
    with open(p, 'w', encoding='utf-8') as f:
        f.write(to_nested_format(obj))

# Bengali translations
bn_BD = {
    "assistant.actions.confirm": "নিশ্চিত করুন",
    "assistant.settings.model.setDefaultModelDesc": "সহায়ক \"{name}\"-এ কোনো ডিফল্ট মডেল সেট করা হয়নি। চালিয়ে যেতে একটি মডেল নির্বাচন করুন।",
    "assistant.settings.model.setDefaultModelTitle": "ডিফল্ট মডেল সেট করুন",
    "nav.openclawTerminal": "টার্মিনাল",
    "nav.skillmarket": "দক্ষতা বাজার",
    "openclawTerminal.loading": "লোড হচ্ছে...",
    "openclawTerminal.tools": "সরঞ্জাম",
    "settings.chatwiki.cloudVersion": "ক্লাউড সংস্করণ",
    "settings.chatwiki.openSourceLoginHint": "বর্তমান অ্যাকাউন্টটি ওপেন সোর্স সংস্করণ অ্যাকাউন্ট এবং chatwiki-এর নিজস্ব মডেল এবং ক্রেডিট ব্যবহার সমর্থন করে না",
    "settings.chatwiki.openSourceVersion": "ওপেন সোর্স সংস্করণ",
    "settings.chatwiki.openSourceVersionHint": "শুধুমাত্র জ্ঞান ভিত্তি/অ্যাপ সিঙ্ক্রোনাইজেশন সমর্থন করে",
    "settings.chatwiki.openSourceVersionLogin": "ওপেন সোর্স সংস্করণ অ্যাকাউন্ট",
    "settings.chatwiki.openclawDescription": "chatwiki-তে অনুমোদন দিন এবং বাইন্ড করুন, chatwiki-এর নিজস্ব মডেল এবং অ্যাকাউন্ট ক্রেডিট ব্যবহার করুন",
    "settings.chatwiki.switchBinding": "বাইন্ডিং পরিবর্তন করুন",
    "settings.general.toolchain.openclaw.description": "OpenClaw রানটাইম পরিবেশের ওয়ান-ক্লিক ইনস্টল এবং ম্যানেজমেন্ট, এজেন্ট ওয়ার্কফ্লো এবং টুলচেইন ক্ষমতা সমর্থন করে।",
    "settings.menu.runtimeEnvironment": "রানটাইম পরিবেশ",
    "settings.openclawRuntime.gatewayLog": "গেটওয়ে লগ",
    "settings.openclawRuntime.resetConfirmDesc": "এই অপারেশন OpenClaw-এর সমস্ত ডেটা স্টোরেজ ফোল্ডার মুছে দেবে, যার মধ্যে প্লাগইন, কথোপকথন ইতিহাস, চ্যানেল কনফিগারেশন ইত্যাদি রয়েছে। এই ক্রিয়া পূর্বাবস্থায় ফেরানো যাবে না। চালিয়ে যেতে চান?",
    "settings.openclawRuntime.resetConfirmTitle": "ফ্যাক্টরি রিসেট নিশ্চিত করুন",
    "settings.openclawRuntime.resetFailed": "ফ্যাক্টরি রিসেট ব্যর্থ",
    "settings.openclawRuntime.resetToFactory": "ফ্যাক্টরি রিসেট",
    "settings.openclawRuntime.resetToFactoryHint": "OpenClaw ডেটা ডিরেক্টরি মুছে দিন এবং অ্যাপটি পুনরায় চালু করুন, পরিবেশের অস্বাভাবিকতার ক্ষেত্রে প্রযোজ্য",
    "settings.openclawRuntime.viewGatewayLog": "লগ দেখুন",
    "settings.openclawRuntime.waitingForLog": "লগ আউটপুট জন্য অপেক্ষা করছি...",
    "settings.runtimeEnvironment.installNow": "ইনস্টল করুন",
    "settings.runtimeEnvironment.installedHint": "OpenClaw রানটাইম ইনস্টল করা আছে। অস্থায়ীভাবে ব্যবহার না করলে, বাম ক্লিক করে নিষ্ক্রিয় করতে পারেন। পরে যেতে পারবেন",
    "settings.runtimeEnvironment.later": "একটু অপেক্ষা করুন",
    "settings.runtimeEnvironment.managerSuffix": "এ নিষ্ক্রিয় করুন",
    "settings.runtimeEnvironment.managerText": "OpenClaw ম্যানেজার",
    "settings.runtimeEnvironment.notInstalledHint": "OpenClaw রানটাইম এখনও ইনস্টল করা হয়নি। প্রথমে ইনস্টলেশন সম্পন্ন করুন।",
    "settings.runtimeEnvironment.pause": "বিরতি দিন",
    "settings.runtimeEnvironment.pauseFailed": "বিরতি ব্যর্থ",
    "settings.runtimeEnvironment.paused": "ডাউনলোড বিরত",
    "settings.runtimeEnvironment.startUsing": "ব্যবহার শুরু করুন",
    "settings.runtimeEnvironment.subtitle": "OpenClaw ব্যবহার করার আগে, OpenClaw রানটাইম পরিবেশ ইনস্টল করুন",
    "settings.runtimeEnvironment.title": "রানটাইম পরিবেশ ইনস্টল করুন",
    "settings.skillMarket.actionInstallSkill": "দক্ষতা ইনস্টল করুন",
    "settings.skillMarket.addSkillChoosePackageDesc": "প্রাসঙ্গিক ডিরেক্টরি খুলুন এবং ডাউনলোড করা দক্ষতা প্যাকেজ রাখুন।",
    "settings.skillMarket.addSkillChoosePackageGuideDesc": "প্রয়োজনীয় দক্ষতা খুঁজুন এবং ইনস্টলেশন ফাইলগুলি প্রাসঙ্গিক ডিরেক্টরিতে ডাউনলোড করুন।",
    "settings.skillMarket.addSkillChoosePackageGuideLabel": "ClawHub দেখুন",
    "settings.skillMarket.addSkillChoosePackageTitle": "দক্ষতা প্যাকেজ নির্বাচন করুন",
    "settings.skillMarket.addSkillDialogTitle": "দক্ষতা যোগ করুন",
    "settings.skillMarket.addSkillHint": "শেয়ার্ড দক্ষতা যোগ করুন",
    "settings.skillMarket.addSkillHintDesc": "খোলা ডিরেক্টরিতে একটি নতুন ফোল্ডার তৈরি করুন এবং শেয়ার্ড দক্ষতা তৈরি করতে SKILL.md ফাইল যোগ করুন।",
    "settings.skillMarket.addSkillHintDescShared": "{dir} ডিরেক্টরিতে একটি নতুন ফোল্ডার তৈরি করুন এবং শেয়ার্ড দক্ষতা তৈরি করতে SKILL.md ফাইল যোগ করুন।",
    "settings.skillMarket.addSkillViaChatDesc": "AI-এর সাথে চ্যাট করুন এবং এটিকে দক্ষতার নাম, বিবরণ এবং বাস্তবায়ন পরিকল্পনা ডিজাইন করতে সাহায্য করতে দিন।",
    "settings.skillMarket.addSkillViaChatGuideDesc": "প্রয়োজনীয় দক্ষতা খুঁজুন, ইনস্টলেশন ইঙ্গিত কপি করুন এবং এজেন্টে পাঠান।",
    "settings.skillMarket.addSkillViaChatGuideLabel": "SkillHub দেখুন",
    "settings.skillMarket.addSkillViaChatTitle": "চ্যাটের মাধ্যমে তৈরি করুন",
    "settings.skillMarket.agentNone": "কোনোটি নয়",
    "settings.skillMarket.agentWorkspaceDirHint": "অনুগ্রহ করে প্রথমে একটি এজেন্ট নির্বাচন করুন",
    "settings.skillMarket.agentWorkspaceDirLoading": "ডিরেক্টরি লোড হচ্ছে...",
    "settings.skillMarket.badgeBuiltIn": "বিল্ট-ইন দক্ষতা",
    "settings.skillMarket.badgeWorkspace": "ওয়ার্কস্পেস",
    "settings.skillMarket.builtInCannotUninstall": "বিল্ট-ইন দক্ষতা আনইনস্টল করা যায় না",
    "settings.skillMarket.builtInUninstallHint": "এই দক্ষতাটি একটি বিল্ট-ইন দক্ষতা এবং সফ্টওয়্যারের সাথে সরবরাহ করা হয়। এটি আনইনস্টল করা যায় না।",
    "settings.skillMarket.deleteConfirm": "দক্ষতা আনইনস্টল করতে চান",
    "settings.skillMarket.disable": "নিষ্ক্রিয় করুন",
    "settings.skillMarket.enable": "সক্রিয় করুন",
    "settings.skillMarket.filePreviewNA": "রিমোট ফাইল প্রিভিউ অস্থায়ীভাবে অনুপলব্ধ",
    "settings.skillMarket.filterAll": "সব",
    "settings.skillMarket.installConfirmDescription": "{skill} {target}-এ ইনস্টল করতে চান?",
    "settings.skillMarket.installDescription": "দক্ষতা ইনস্টল করার জন্য ডিরেক্টরি নির্বাচন করুন",
    "settings.skillMarket.installFailed": "ইনস্টলেশন ব্যর্থ",
    "settings.skillMarket.installSuccess": "সফলভাবে ইনস্টল হয়েছে",
    "settings.skillMarket.installTitle": "ইনস্টলেশন টার্গেট নির্বাচন করুন",
    "settings.skillMarket.introTitle": "দক্ষতার পরিচিতি",
    "settings.skillMarket.listHeading": "দক্ষতা বাজার",
    "settings.skillMarket.listSubheading": "রিমোট থেকে AI দক্ষতা স্থানীয়ভাবে ডাউনলোড করুন",
    "settings.skillMarket.loadFailed": "লোড ব্যর্থ, পরে আবার চেষ্টা করুন",
    "settings.skillMarket.loadMore": "আরো লোড করুন",
    "settings.skillMarket.loadingAgents": "এজেন্ট লোড হচ্ছে...",
    "settings.skillMarket.loadingTargets": "টার্গেট লোড হচ্ছে...",
    "settings.skillMarket.noSkills": "কোনো দক্ষতা নেই",
    "settings.skillMarket.noSkillsHint": "অন্যান্য বিভাগ বা সার্চ কীওয়ার্ড চেষ্টা করুন",
    "settings.skillMarket.openDir": "ডিরেক্টরি খুলুন",
    "settings.skillMarket.openMainWorkspaceSkillsDir": "প্রধান ওয়ার্কস্পেস দক্ষতা ডিরেক্টরি খুলুন",
    "settings.skillMarket.refreshCta": "রিফ্রেশ করুন",
    "settings.skillMarket.scopeAgentOption": "এজেন্ট দক্ষতা",
    "settings.skillMarket.scopeSharedOption": "শেয়ার্ড দক্ষতা",
    "settings.skillMarket.searchPlaceholder": "দক্ষতা খুঁজুন...",
    "settings.skillMarket.securityVerifiedHint": "নিরাপত্তা এবং কমপ্লায়েন্স যাচাই করা হয়েছে, কোনো ম্যালিসিয়াস কোড বা ডেটা লিকের ঝুঁকি নেই।",
    "settings.skillMarket.selectTarget": "ইনস্টলেশন টার্গেট",
    "settings.skillMarket.statusInstalled": "ইনস্টল করা",
    "settings.skillMarket.statusInstalling": "ইনস্টল হচ্ছে",
    "settings.skillMarket.syncFailedDescription": "ব্যাকএন্ড সার্ভারে সংযোগ করতে অক্ষম",
    "settings.skillMarket.syncFailedTitle": "দক্ষতা বাজার সিঙ্ক ব্যর্থ",
    "settings.skillMarket.tabBrowse": "দক্ষতা লাইব্রেরি",
    "settings.skillMarket.tabInstalled": "আমার দক্ষতা",
    "settings.skillMarket.targetAgentWorkspace": "এজেন্ট ওয়ার্ক ডিরেক্টরি",
    "settings.skillMarket.targetLocal": "স্থানীয় ডিরেক্টরি",
    "settings.skillMarket.targetOpenClawShared": "OpenClaw শেয়ার্ড দক্ষতা",
    "settings.skillMarket.targetOpenClawWorkspace": "OpenClaw ওয়ার্কস্পেস",
    "settings.skillMarket.title": "দক্ষতা বাজার",
    "settings.skillMarket.toggleFailed": "টগল ব্যর্থ, পরে আবার চেষ্টা করুন",
    "settings.skillMarket.uninstallFailed": "আনইনস্টল ব্যর্থ",
    "settings.skillMarket.uninstallSuccess": "সফলভাবে আনইনস্টল হয়েছে",
    "settings.skillMarket.usageTitle": "কীভাবে ব্যবহার করবেন?",
    "settings.skillMarket.viewDetail": "বিস্তারিত দেখুন",
    "settings.skills.added": "যোগ করা হয়েছে",
}

# Import
target_file = "bn-BD.ts"
target_path = os.path.join(FRONTEND_LOCALES_DIR, target_file)
data = load_ts_file(target_path)
count = 0
for key, value in bn_BD.items():
    if key in data:
        data[key] = value
        count += 1

save_ts_file(target_path, data)
print(f"Imported {count} translations to {target_file}")
