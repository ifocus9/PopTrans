"""
测试 llama.cpp 集成
"""

import sys
import os
import time

# 添加当前目录到 Python 路径
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from translator import Translator

def test_translation():
    """测试翻译功能"""
    print("=" * 50)
    print("测试 llama.cpp 集成")
    print("=" * 50)
    
    # 创建翻译器实例
    translator = Translator()
    
    # 测试状态
    print(f"初始状态: {translator.status}")
    print(f"就绪状态: {translator.ready}")
    
    # 定义回调函数
    def on_ready(success):
        if success:
            print("✓ 翻译引擎初始化成功")
        else:
            print("✗ 翻译引擎初始化失败")
    
    def on_status(message):
        print(f"状态更新: {message}")
    
    # 初始化翻译引擎
    print("\n正在初始化翻译引擎...")
    translator.setup(on_ready=on_ready, on_status=on_status)
    
    # 等待初始化完成
    while not translator.ready and "失败" not in translator.status:
        time.sleep(1)
        print(f"等待中... 当前状态: {translator.status}")
    
    if not translator.ready:
        print("翻译引擎初始化失败，无法进行测试")
        return
    
    # 测试翻译
    test_cases = [
        ("Hello, how are you?", "英→中"),
        ("今天天气真好", "中→英"),
        ("This is a test sentence.", "英→中"),
        ("我喜欢编程", "中→英"),
    ]
    
    print("\n" + "=" * 50)
    print("开始翻译测试")
    print("=" * 50)
    
    for text, expected_direction in test_cases:
        print(f"\n原文: {text}")
        print(f"期望方向: {expected_direction}")
        
        result, error = translator.translate(text)
        
        if error:
            print(f"错误: {error}")
        else:
            print(f"译文: {result}")
        
        print("-" * 30)
    
    print("\n测试完成!")

if __name__ == "__main__":
    test_translation()