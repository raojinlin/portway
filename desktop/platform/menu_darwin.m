#import <Cocoa/Cocoa.h>

extern void stmMenuAction(int action);

@interface STMMenuTarget : NSObject
- (void)activate:(NSMenuItem *)sender;
@end

@implementation STMMenuTarget
- (void)activate:(NSMenuItem *)sender { stmMenuAction((int)sender.tag); }
@end

static NSStatusItem *statusItem;
static STMMenuTarget *menuTarget;
static NSMenuItem *summaryItem;
static NSMenuItem *trafficItem;
static NSMenuItem *emptyItem;
static NSMutableDictionary<NSString *, NSMenuItem *> *lineItems;
static NSString *trayName;

static NSString *stmMenuText(NSString *value) {
    NSString *singleLine = [[value componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet] componentsJoinedByString:@" "];
    if (singleLine.length <= 120) return singleLine;
    NSRange range = [singleLine rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, 120)];
    return [[singleLine substringWithRange:range] stringByAppendingString:@"\u2026"];
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
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (statusItem == nil || statusItem.button == nil) { result = 2; return; }
        statusItem.visible = YES;
        statusItem.button.toolTip = name;
        if (@available(macOS 11.0, *)) {
            NSImage *image = [NSImage imageWithSystemSymbolName:@"arrow.left.arrow.right" accessibilityDescription:name];
            image.size = NSMakeSize(18, 18);
            image.template = YES;
            statusItem.button.image = image;
        }
        if (statusItem.button.image == nil) statusItem.button.title = name;
        NSMenu *menu = [NSMenu new];
        menu.autoenablesItems = NO;
        summaryItem = [[NSMenuItem alloc] initWithTitle:name action:nil keyEquivalent:@""];
        summaryItem.enabled = NO;
        [menu addItem:summaryItem];
        trafficItem = [[NSMenuItem alloc] initWithTitle:@"\u6b63\u5728\u8bfb\u53d6\u6d41\u91cf\u2026" action:nil keyEquivalent:@""];
        trafficItem.enabled = NO;
        [menu addItem:trafficItem];
        [menu addItem:[NSMenuItem separatorItem]];
        emptyItem = [[NSMenuItem alloc] initWithTitle:@"\u6682\u65e0\u7ebf\u8def" action:nil keyEquivalent:@""];
        emptyItem.enabled = NO;
        [menu addItem:emptyItem];
        lineItems = [NSMutableDictionary new];
        [menu addItem:[NSMenuItem separatorItem]];
        NSArray<NSString *> *titles = @[
            [@"Open " stringByAppendingString:name], @"Open Configuration Folder", [@"Quit " stringByAppendingString:name]
        ];
        for (NSInteger i = 0; i < titles.count; i++) {
            NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titles[i] action:@selector(activate:) keyEquivalent:@""];
            item.target = menuTarget;
            item.tag = i + 1;
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
        NSMenu *menu = statusItem.menu;
        summaryItem.title = snapshot[@"summary"];
        trafficItem.title = snapshot[@"traffic"];
        statusItem.button.toolTip = [NSString stringWithFormat:@"%@\n%@\n%@", trayName, summaryItem.title, trafficItem.title];
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
            item.toolTip = name;
            NSArray<NSString *> *details = line[@"details"];
            while (item.submenu.numberOfItems > details.count) [item.submenu removeItemAtIndex:item.submenu.numberOfItems - 1];
            while (item.submenu.numberOfItems < details.count) {
                NSMenuItem *detail = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
                detail.enabled = NO;
                [item.submenu addItem:detail];
            }
            for (NSInteger i = 0; i < details.count; i++) {
                NSMenuItem *detail = [item.submenu itemAtIndex:i];
                detail.title = stmMenuText(details[i]);
                detail.toolTip = details[i];
            }
        }
    });
}

void stmTrayStop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem != nil) [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
        statusItem = nil;
        summaryItem = nil;
        trafficItem = nil;
        emptyItem = nil;
        lineItems = nil;
        trayName = nil;
        menuTarget = nil;
    });
}
