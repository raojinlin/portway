#include <assert.h>
#include "../menu_darwin.m"

static NSString *lastName;
static BOOL lastEnabled;
static BOOL lastHistory;
void stmConnectionsAction(const char *name, int history) {
    lastName = [NSString stringWithUTF8String:name];
    lastHistory = history != 0;
}
void stmMenuAction(int action) {}
void stmTunnelAction(const char *name, int enabled) {
    lastName = [NSString stringWithUTF8String:name];
    lastEnabled = enabled != 0;
}

int main(void) {
    @autoreleasepool {
        menuTarget = [STMMenuTarget new];
        NSMenu *menu = [NSMenu new];
        menu.autoenablesItems = NO;
        [menu addItem:[[NSMenuItem alloc] initWithTitle:@"Details" action:nil keyEquivalent:@""]];
        NSDictionary *labels = @{@"startTunnel": @"Start Tunnel", @"stopTunnel": @"Stop Tunnel", @"working": @"Working...",
            @"copyProxy": @"Copy Proxy URL", @"copyCommand": @"Copy Proxy Command", @"connections": @"View Connections"};
        NSMutableDictionary *line = [@{@"name": @"line / test", @"enabled": @NO, @"busy": @NO} mutableCopy];
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 4);
        NSMenuItem *connections = [menu itemAtIndex:3];
        assert(connections.enabled && connections.image != nil && [connections.title isEqualToString:@"View Connections"]);
        [menuTarget viewConnections:connections];
        assert([lastName isEqualToString:line[@"name"]] && !lastHistory);
        NSMenuItem *toggle = [menu itemAtIndex:2];
        assert([toggle.title isEqualToString:@"Start Tunnel"] && [toggle.toolTip isEqualToString:@"Start Tunnel"] && toggle.enabled && toggle.image != nil);
        assert([toggle.accessibilityLabel isEqualToString:@"Start Tunnel"]);
        [menuTarget setTunnel:toggle];
        assert([lastName isEqualToString:line[@"name"]] && lastEnabled && !toggle.enabled);

        line[@"enabled"] = @YES;
        line[@"showHistory"] = @YES;
        line[@"proxyURL"] = @"socks5h://127.0.0.1:1080";
        line[@"proxyCommand"] = @"export ALL_PROXY='socks5h://127.0.0.1:1080'";
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 6);
        [menuTarget viewConnections:connections];
        assert(lastHistory);
        assert([toggle.title isEqualToString:@"Stop Tunnel"] && [toggle.toolTip isEqualToString:@"Stop Tunnel"] && toggle.enabled && toggle.image != nil);
        assert([menu itemAtIndex:4].image != nil && [[menu itemAtIndex:4].title isEqualToString:@"Copy Proxy URL"]);
        assert([menu itemAtIndex:5].image != nil && [[menu itemAtIndex:5].title isEqualToString:@"Copy Proxy Command"]);
        assert([[[menu itemAtIndex:4] representedObject] isEqual:line[@"proxyURL"]]);
        [menuTarget setTunnel:toggle];
        assert(!lastEnabled);

        line[@"busy"] = @YES;
        stmUpdateLineActions(menu, line, labels);
        assert([toggle.title isEqualToString:@"Working..."] && !toggle.enabled && toggle.image != nil);
        line[@"busy"] = @NO;
        line[@"enabled"] = @NO;
        [line removeObjectForKey:@"proxyURL"];
        [line removeObjectForKey:@"proxyCommand"];
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 4 && [toggle.toolTip isEqualToString:@"Start Tunnel"]);
        [menuTarget viewConnections:connections];
        assert(connections.enabled && lastHistory);

        NSView *details = [NSView new];
        for (NSArray *rows in @[
            @[@"SOCKS5 监听：127.0.0.1:1080", @"SSH 跳板：drop", @"目标：由客户端指定"],
            @[@"SOCKS5: 127.0.0.1:1080", @"SSH gateway: drop", @"Target: Specified by client"]
        ]) {
            stmUpdateDetailsView(details, rows);
            CGFloat valueX = -1;
            for (NSInteger i = 0; i < rows.count; i++) {
                NSTextField *key = details.subviews[i * 2];
                NSTextField *value = details.subviews[i * 2 + 1];
                assert(key.cell.cellSize.width <= key.frame.size.width);
                assert(value.cell.cellSize.width <= value.frame.size.width);
                if (valueX >= 0) assert(value.frame.origin.x == valueX);
                valueX = value.frame.origin.x;
            }
        }
        puts("Menu actions: start, stop, busy and proxy visibility OK.");
    }
    return 0;
}
