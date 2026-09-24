#import <Cocoa/Cocoa.h>
#import <CoreText/CoreText.h>

extern void stmMenuAction(int action);
extern void stmTunnelAction(const char *name, int enabled);

@interface STMMenuTarget : NSObject
@property(nonatomic, copy) NSString *name;
@property(nonatomic, copy) NSDictionary *labels;
- (void)activate:(NSMenuItem *)sender;
- (void)showAbout:(id)sender;
- (void)copyProxy:(NSMenuItem *)sender;
- (void)setTunnel:(NSMenuItem *)sender;
@end

@implementation STMMenuTarget
- (void)activate:(NSMenuItem *)sender { stmMenuAction((int)sender.tag); }
- (void)setTunnel:(NSMenuItem *)sender {
    NSDictionary *action = sender.representedObject;
    if (![action isKindOfClass:NSDictionary.class] || ![action[@"name"] isKindOfClass:NSString.class]) return;
    sender.enabled = NO;
    stmTunnelAction([action[@"name"] UTF8String], [action[@"enabled"] boolValue]);
}
- (void)copyProxy:(NSMenuItem *)sender {
    NSString *text = sender.representedObject;
    if (![text isKindOfClass:NSString.class] || text.length == 0) return;
    NSPasteboard *pasteboard = NSPasteboard.generalPasteboard;
    [pasteboard clearContents];
    if (![pasteboard setString:text forType:NSPasteboardTypeString]) NSBeep();
}
- (void)showAbout:(id)sender {
    NSAlert *alert = [NSAlert new];
    alert.messageText = self.name;
    NSString *version = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"";
    alert.informativeText = [NSString stringWithFormat:@"%@ %@\n\n%@", self.labels[@"version"], version, self.labels[@"aboutDescription"]];
    [alert addButtonWithTitle:self.labels[@"ok"]];
    alert.window.level = NSFloatingWindowLevel;
    [alert runModal];
}
@end

static NSStatusItem *statusItem;
static STMMenuTarget *menuTarget;
static NSImage *traySymbol;
static NSMenuItem *emptyItem;
static NSMutableDictionary<NSString *, NSMenuItem *> *lineItems;
static NSMutableDictionary<NSString *, NSImage *> *stateImages;
static NSString *trayName;

int stmSystemChinese(void) {
    @autoreleasepool {
        for (NSString *language in NSLocale.preferredLanguages) {
            if ([language hasPrefix:@"zh"]) return 1;
            if ([language hasPrefix:@"en"]) return 0;
        }
        return 0;
    }
}

static NSImage *stmStatusImage(NSString *text, NSImage *symbol) {
    NSArray<NSString *> *rates = [text componentsSeparatedByString:@"\n"];
    if (rates.count != 2) rates = @[@"--", @"--"];
    NSDictionary *attributes = @{
        NSFontAttributeName: [NSFont monospacedDigitSystemFontOfSize:8.5 weight:NSFontWeightMedium],
        NSForegroundColorAttributeName: NSColor.blackColor
    };
    const CGFloat height = 20;
    const CGFloat iconWidth = symbol == nil ? 0 : 18;
    const CGFloat textX = iconWidth == 0 ? 0 : iconWidth + 3;
    CGFloat textWidth = 0;
    for (NSString *rate in rates) {
        CTLineRef line = CTLineCreateWithAttributedString((__bridge CFAttributedStringRef)[[NSAttributedString alloc] initWithString:rate attributes:attributes]);
        textWidth = MAX(textWidth, ceil(CTLineGetBoundsWithOptions(line, kCTLineBoundsUseGlyphPathBounds).size.width));
        CFRelease(line);
    }
    NSSize size = NSMakeSize(textX + textWidth + 1, height);
    // A single template image keeps the icon and both rows centered together,
    // without NSButtonCell's single-title baseline or multiline clipping.
    NSImage *image = [NSImage imageWithSize:size flipped:NO drawingHandler:^BOOL(NSRect bounds) {
        NSRectClip(bounds);
        [symbol drawInRect:NSMakeRect(0, (height - iconWidth) / 2, iconWidth, iconWidth)];
        CGContextRef context = NSGraphicsContext.currentContext.CGContext;
        CGContextSetTextMatrix(context, CGAffineTransformIdentity);
        for (NSUInteger i = 0; i < rates.count; i++) {
            NSAttributedString *value = [[NSAttributedString alloc] initWithString:rates[i] attributes:attributes];
            CTLineRef line = CTLineCreateWithAttributedString((__bridge CFAttributedStringRef)value);
            CGRect ink = CTLineGetBoundsWithOptions(line, kCTLineBoundsUseGlyphPathBounds);
            CGFloat centerY = i == 0 ? height * 0.75 : height * 0.25;
            CGContextSetTextPosition(context, size.width - 1 - CGRectGetMaxX(ink), centerY - CGRectGetMidY(ink));
            CTLineDraw(line, context);
            CFRelease(line);
        }
        return YES;
    }];
    image.template = YES;
    return image;
}

static void stmUpdateStatusText(NSString *text) {
    statusItem.button.image = stmStatusImage(text, traySymbol);
}

static void stmTranslateMenu(NSMenu *menu, NSDictionary *labels) {
    NSDictionary *actions = @{@"About": @"about", @"showAbout:": @"about", @"hide:": @"hide", @"hideOtherApplications:": @"hideOthers",
        @"unhideAllApplications:": @"showAll", @"Quit": @"quit", @"undo:": @"undo", @"redo:": @"redo",
        @"cut:": @"cut", @"copy:": @"copy", @"paste:": @"paste", @"pasteAsRichText:": @"pasteStyle",
        @"delete:": @"delete", @"selectAll:": @"selectAll", @"startSpeaking:": @"speak", @"stopSpeaking:": @"stopSpeaking",
        @"performMiniaturize:": @"minimize", @"performZoom:": @"zoom", @"enterFullScreenMode:": @"fullscreen"};
    NSDictionary *submenus = @{@"undo:": @"edit", @"performMiniaturize:": @"window", @"startSpeaking:": @"speech"};
    for (NSMenuItem *item in menu.itemArray) {
        if (item.submenu != nil) {
            for (NSMenuItem *child in item.submenu.itemArray) {
                if (child.action == NULL) continue;
                NSString *key = submenus[NSStringFromSelector(child.action)];
                if (key != nil && [labels[key] isKindOfClass:NSString.class]) {
                    item.title = labels[key];
                    item.submenu.title = labels[key];
                    break;
                }
            }
            stmTranslateMenu(item.submenu, labels);
        }
        NSString *key = item.action == NULL ? nil : actions[NSStringFromSelector(item.action)];
        NSString *title = key == nil ? nil : labels[key];
        if ([title isKindOfClass:NSString.class]) item.title = title;
        if ([key isEqualToString:@"about"]) {
            item.action = @selector(showAbout:);
            item.target = menuTarget;
        }
    }
}

static void stmUpdateMenuLanguage(NSDictionary *labels) {
    if (![labels isKindOfClass:NSDictionary.class]) return;
    menuTarget.labels = labels;
    stmTranslateMenu(NSApp.mainMenu, labels);
    NSArray *keys = @[@"open", @"directory", @"quit", @"logs"];
    for (NSMenuItem *item in statusItem.menu.itemArray) {
        if (item.tag < 1 || item.tag > 4) continue;
        NSString *title = labels[keys[item.tag - 1]];
        if ([title isKindOfClass:NSString.class]) item.title = title;
    }
    if ([labels[@"empty"] isKindOfClass:NSString.class]) emptyItem.title = labels[@"empty"];
}

static NSString *stmMenuText(NSString *value) {
    NSString *singleLine = [[value componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet] componentsJoinedByString:@" "];
    if (singleLine.length <= 120) return singleLine;
    NSRange range = [singleLine rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, 120)];
    return [[singleLine substringWithRange:range] stringByAppendingString:@"\u2026"];
}

static NSImage *stmStateImage(NSString *state) {
    if (![state isKindOfClass:NSString.class]) state = @"stopped";
    NSImage *cached = stateImages[state];
    if (cached != nil) return cached;
    NSColor *color = NSColor.systemGrayColor;
    if ([state isEqualToString:@"running"]) color = NSColor.systemGreenColor;
    else if ([state isEqualToString:@"starting"]) color = NSColor.systemOrangeColor;
    else if ([state isEqualToString:@"error"]) color = NSColor.systemRedColor;
    NSImage *image = [[NSImage alloc] initWithSize:NSMakeSize(10, 10)];
    [image lockFocus];
    [color setFill];
    [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(1, 1, 8, 8)] fill];
    [image unlockFocus];
    image.template = NO;
    stateImages[state] = image;
    return image;
}

static void stmUpdateDetailsView(NSView *view, NSArray<NSString *> *details) {
    const CGFloat width = 300;
    const CGFloat rowHeight = 22;
    const CGFloat padding = 10;
    CGFloat height = padding * 2 + rowHeight * details.count;
    view.frame = NSMakeRect(0, 0, width, height);
    while (view.subviews.count > details.count * 2) [view.subviews.lastObject removeFromSuperview];
    while (view.subviews.count < details.count * 2) {
        NSTextField *label = [NSTextField labelWithString:@""];
        label.textColor = NSColor.labelColor;
        [view addSubview:label];
    }
    for (NSInteger i = 0; i < details.count; i++) {
        NSString *text = stmMenuText(details[i]);
        NSRange separator = [text rangeOfString:@"\uff1a"];
        if (separator.location == NSNotFound) separator = [text rangeOfString:@": "];
        NSString *key = separator.location == NSNotFound ? @"" : [text substringToIndex:separator.location];
        NSString *value = separator.location == NSNotFound ? text : [text substringFromIndex:NSMaxRange(separator)];
        CGFloat y = height - padding - 17 - rowHeight * i;
        NSTextField *keyLabel = (NSTextField *)view.subviews[i * 2];
        keyLabel.stringValue = key;
        CGFloat keyWidth = [text containsString:@"\uff1a"] ? 66 : 86;
        keyLabel.frame = NSMakeRect(14, y, keyWidth, 17);
        keyLabel.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
        NSTextField *valueLabel = (NSTextField *)view.subviews[i * 2 + 1];
        valueLabel.stringValue = value;
        valueLabel.frame = NSMakeRect(18 + keyWidth, y, width - 32 - keyWidth, 17);
        valueLabel.font = [NSFont systemFontOfSize:12 weight:NSFontWeightRegular];
        valueLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        valueLabel.toolTip = details[i];
    }
}

void stmSetAppearance(int dark, int followSystem) {
    dispatch_async(dispatch_get_main_queue(), ^{
        // Wails' macOS runtime theme setters are no-ops; AppKit needs an explicit appearance.
        // nil restores inheritance, including WebKit's prefers-color-scheme notifications.
        NSApp.appearance = followSystem ? nil : [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
        for (NSWindow *window in NSApp.windows) {
            if (!window.canBecomeMainWindow) continue;
            // Set this directly: Wails' transparent-titlebar option also extends content under the controls.
            window.titlebarAppearsTransparent = YES;
            if (@available(macOS 11.0, *)) window.titlebarSeparatorStyle = NSTitlebarSeparatorStyleNone;
        }
    });
}

int stmTrayStart(const char *value) {
    NSString *name = [NSString stringWithUTF8String:value];
    __block int result = 0;
    void (^start)(void) = ^{
        NSMenuItem *applicationItem = NSApp.mainMenu.itemArray.firstObject;
        NSMenu *applicationMenu = applicationItem.submenu;
        if (applicationMenu == nil) { result = 1; return; }
        // Preserve Wails' native actions and shortcuts, but not its inferred/cached app name.
        applicationItem.title = name;
        applicationMenu.title = name;
        for (NSMenuItem *item in applicationMenu.itemArray) {
            if (item.action == NULL) continue;
            NSString *action = NSStringFromSelector(item.action);
            if ([action isEqualToString:@"About"]) item.title = [@"\u5173\u4e8e " stringByAppendingString:name];
            if ([action isEqualToString:@"hide:"]) item.title = [@"Hide " stringByAppendingString:name];
            if ([action isEqualToString:@"Quit"]) item.title = [@"Quit " stringByAppendingString:name];
        }
        if (statusItem != nil) return;
        trayName = name;
        menuTarget = [STMMenuTarget new];
        menuTarget.name = name;
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (statusItem == nil || statusItem.button == nil) { result = 2; return; }
        statusItem.visible = YES;
        statusItem.button.toolTip = name;
        if (@available(macOS 11.0, *)) {
            traySymbol = [NSImage imageWithSystemSymbolName:@"arrow.left.arrow.right" accessibilityDescription:name];
            traySymbol.size = NSMakeSize(18, 18);
        }
        statusItem.button.title = @"";
        statusItem.button.imagePosition = NSImageOnly;
        statusItem.button.imageScaling = NSImageScaleNone;
        stmUpdateStatusText(@"--\n--");
        NSMenu *menu = [NSMenu new];
        menu.autoenablesItems = NO;
        emptyItem = [[NSMenuItem alloc] initWithTitle:@"\u6682\u65e0\u7ebf\u8def" action:nil keyEquivalent:@""];
        emptyItem.enabled = NO;
        [menu addItem:emptyItem];
        lineItems = [NSMutableDictionary new];
        stateImages = [NSMutableDictionary new];
        [menu addItem:[NSMenuItem separatorItem]];
        NSArray<NSString *> *titles = @[
            [@"Open " stringByAppendingString:name], @"Open Logs", @"Open Configuration Folder", [@"Quit " stringByAppendingString:name]
        ];
        NSArray<NSNumber *> *tags = @[@1, @4, @2, @3];
        for (NSInteger i = 0; i < titles.count; i++) {
            NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titles[i] action:@selector(activate:) keyEquivalent:@""];
            item.target = menuTarget;
            item.tag = tags[i].integerValue;
            [menu addItem:item];
        }
        statusItem.menu = menu;
    };
    // OnStartup runs on a Go worker. Wait for AppKit, not for webview DomReady,
    // and report success only after the menus actually exist.
    if ([NSThread isMainThread]) start();
    else dispatch_sync(dispatch_get_main_queue(), start);
    return result;
}

static void stmUpdateLineActions(NSMenu *menu, NSDictionary *line, NSDictionary *labels) {
    if (menu.numberOfItems == 1) {
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *toggle = [[NSMenuItem alloc] initWithTitle:@"" action:@selector(setTunnel:) keyEquivalent:@""];
        toggle.target = menuTarget;
        [menu addItem:toggle];
    }
    BOOL enabled = [line[@"enabled"] boolValue];
    BOOL busy = [line[@"busy"] boolValue];
    NSMenuItem *toggle = [menu itemAtIndex:2];
    toggle.title = labels[busy ? @"working" : enabled ? @"stopTunnel" : @"startTunnel"];
    toggle.enabled = !busy;
    toggle.representedObject = @{@"name": line[@"name"], @"enabled": @(!enabled)};
    if (@available(macOS 11.0, *)) {
        toggle.image = [NSImage imageWithSystemSymbolName:enabled ? @"stop.fill" : @"play.fill" accessibilityDescription:nil];
    }
    NSString *proxyURL = line[@"proxyURL"];
    BOOL hasProxy = [proxyURL isKindOfClass:NSString.class] && proxyURL.length > 0;
    if (!hasProxy) {
        while (menu.numberOfItems > 3) [menu removeItemAtIndex:3];
    } else {
        if (menu.numberOfItems == 3) {
            for (NSInteger i = 0; i < 2; i++) {
                NSMenuItem *copy = [[NSMenuItem alloc] initWithTitle:@"" action:@selector(copyProxy:) keyEquivalent:@""];
                copy.target = menuTarget;
                [menu addItem:copy];
            }
        }
        NSMenuItem *copyURL = [menu itemAtIndex:3];
        copyURL.title = labels[@"copyProxy"];
        copyURL.representedObject = proxyURL;
        copyURL.toolTip = proxyURL;
        NSMenuItem *copyCommand = [menu itemAtIndex:4];
        copyCommand.title = labels[@"copyCommand"];
        copyCommand.representedObject = line[@"proxyCommand"];
        copyCommand.toolTip = line[@"proxyCommand"];
    }
}

void stmTrayUpdate(const char *value) {
    NSDictionary *snapshot;
    // This entry point runs on a Go worker, outside AppKit's autorelease pool.
    @autoreleasepool {
        NSData *data = [[NSString stringWithUTF8String:value] dataUsingEncoding:NSUTF8StringEncoding];
        snapshot = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
    }
    if (![snapshot isKindOfClass:NSDictionary.class]) return;
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem == nil) return;
        NSString *statusText = snapshot[@"statusText"];
        if ([statusText isKindOfClass:NSString.class] && statusText.length > 0) {
            stmUpdateStatusText(statusText);
        }
        stmUpdateMenuLanguage(snapshot[@"labels"]);
        NSMenu *menu = statusItem.menu;
        statusItem.button.toolTip = trayName;
        NSArray *rates = [statusText componentsSeparatedByString:@"\n"];
        NSDictionary *labels = snapshot[@"labels"];
        if (rates.count == 2) {
            statusItem.button.toolTip = [statusItem.button.toolTip stringByAppendingFormat:@"\n%@: %@\n%@: %@",
                labels[@"upload"], rates[0], labels[@"download"], rates[1]];
            statusItem.button.accessibilityLabel = statusItem.button.toolTip;
        }
        NSArray *lines = snapshot[@"lines"];
        emptyItem.hidden = lines.count != 0;
        NSMutableSet *names = [NSMutableSet new];
        for (NSDictionary *line in lines) [names addObject:line[@"name"]];
        for (NSString *name in lineItems.allKeys) {
            if (![names containsObject:name]) {
                [menu removeItem:lineItems[name]];
                [lineItems removeObjectForKey:name];
            }
        }
        NSInteger index = [menu indexOfItem:emptyItem] + 1;
        for (NSDictionary *line in lines) {
            NSString *name = line[@"name"];
            NSMenuItem *item = lineItems[name];
            if (item == nil) {
                item = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
                item.submenu = [NSMenu new];
                item.submenu.autoenablesItems = NO;
                item.enabled = YES;
                lineItems[name] = item;
            }
            // Update in place so an open submenu keeps its selection on each sample.
            if ([menu indexOfItem:item] != index) {
                if (item.menu == menu) [menu removeItem:item];
                [menu insertItem:item atIndex:index];
            }
            index++;
            item.title = line[@"title"];
            item.image = stmStateImage(line[@"state"]);
            item.toolTip = name;
            NSArray<NSString *> *details = line[@"details"];
            NSMenuItem *detail = item.submenu.itemArray.firstObject;
            if (detail == nil) {
                detail = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
                detail.view = [NSView new];
                [item.submenu addItem:detail];
            }
            stmUpdateDetailsView(detail.view, details);
            stmUpdateLineActions(item.submenu, line, labels);
        }
    });
}

void stmTrayStop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem != nil) [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
        statusItem = nil;
        traySymbol = nil;
        emptyItem = nil;
        lineItems = nil;
        stateImages = nil;
        trayName = nil;
        menuTarget = nil;
    });
}
