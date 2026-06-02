#!/bin/bash
# release-module.sh - 发布子模块到 GitHub（自动打 tag）

set -euo pipefail

MODULE="${1:-}"
VERSION="${2:-}"

if [ -z "${MODULE}" ] || [ -z "${VERSION}" ]; then
    echo "用法: $0 <module-path> <version>"
    echo "示例: $0 cache v1.0.0"
    echo "      $0 orm v0.1.0"
    exit 1
fi

# Validate version format (must start with 'v')
if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ 版本格式错误，应为 vX.Y.Z（如 v1.0.0）"
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAG="${MODULE}/${VERSION}"

echo "🏷️  准备发布模块: ${MODULE}"
echo "📦 版本标签: ${TAG}"

# Check if tag already exists  
if git -C "${REPO_ROOT}" rev-parse "${TAG}" >/dev/null ; then        
           
            
                
                    
        
            
                 
                     
            
                  


echo "
       


```bash  
  
  

                                                                   
  
                                                                               
    
                                                                                
                                                            
            
        
                    
                
                      
        
                                          
            
                            
                  
                                 
                   
    
                                              
 
                        


```bash                                     
            
                   
        
                             
            
                                    
             
 
                    
                         




```bash                        
            

                                               
    
                                                           
  
                        
                                      



```                                    


```
                      
  
                                                             
 
                                           



#!/usr/bin/env bash                
set-euo pipefail                  


              




#!/usr/bin/env bash               set-euo pipefail                  ATHENS_URL="${ATHENS_URL:-http://localhost:


EOF